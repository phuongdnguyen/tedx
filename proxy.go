package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"go.temporal.io/api/workflowservice/v1"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/proto"
)

type TemporalProxy struct {
	Server   *http.Server
	Resolver *Resolver
}

func NewTemporalProxy(listenAddr string, registry *VirtualNamespaceRegistry) (*TemporalProxy, error) {
	defaultTargetURLStr := "http://localhost:7233"
	defaultTarget, err := url.Parse(defaultTargetURLStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse default target URL: %w", err)
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			if req.URL.Host == "" {
				req.URL.Host = defaultTarget.Host
				req.Host = defaultTarget.Host
			}
		},
	}

	// Temporal uses gRPC, which requires HTTP/2. Since we aren't using TLS (h2c),
	// we need to configure the proxy's transport to allow HTTP/2 over cleartext TCP.
	proxy.Transport = &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}

	mappings := NewMappings("./mappings.json")
	resolver := NewResolver(mappings)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the RPC call is for StartWorkflowExecution
		if strings.Contains(r.URL.Path, "StartWorkflowExecution") {
			// Read the payload from the request body
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				// gRPC payload structure:
				// 1 byte compressed flag
				// 4 bytes message length
				// N bytes protobuf payload
				fmt.Printf("Intercepted StartWorkflowExecution!\n")
				fmt.Printf("Path: %s\n", r.URL.Path)
				fmt.Printf("Payload Length: %d bytes\n", len(bodyBytes))

				newBodyBytes, err := handleStartWorkflowExecution(r, bodyBytes, resolver, registry)
				if err != nil {
					log.Printf("Error processing StartWorkflowExecution: %v\n", err)
				} else {
					bodyBytes = newBodyBytes
				}

				// Recreate the body so the proxy can still forward it to the upstream server
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			} else {
				log.Printf("Error reading request body: %v\n", err)
			}
		} else if strings.Contains(r.URL.Path, "PollWorkflowTaskQueue") {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				handlePollWorkflowTaskQueue(bodyBytes)
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			} else {
				log.Printf("Error reading request body: %v\n", err)
			}
		}

		// Forward the request to the target Temporal server
		proxy.ServeHTTP(w, r)
	})

	// We wrap our handler in h2c.NewHandler to allow the proxy server itself
	// to accept HTTP/2 connections over plaintext from the Temporal client.
	h2cHandler := h2c.NewHandler(handler, &http2.Server{})

	server := &http.Server{
		Addr:    listenAddr,
		Handler: h2cHandler,
	}

	return &TemporalProxy{
		Server:   server,
		Resolver: resolver,
	}, nil
}

// Start runs the proxy server and blocks until it stops or an error occurs.
func (tp *TemporalProxy) Start() error {
	log.Printf("Starting lightweight proxy on %s \n", tp.Server.Addr)
	return tp.Server.ListenAndServe()
}

// Stop gracefully shuts down the proxy server.
func (tp *TemporalProxy) Stop(ctx context.Context) error {
	log.Printf("Stopping proxy on %s\n", tp.Server.Addr)
	return tp.Server.Shutdown(ctx)
}

func handleStartWorkflowExecution(r *http.Request, bodyBytes []byte, resolver *Resolver, registry *VirtualNamespaceRegistry) ([]byte, error) {
	if len(bodyBytes) <= 5 {
		return nil, fmt.Errorf("payload too short to be valid gRPC")
	}

	var pbPayload []byte
	isCompressed := bodyBytes[0] == 1
	if isCompressed {
		gz, err := gzip.NewReader(bytes.NewReader(bodyBytes[5:]))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		pbPayload, err = io.ReadAll(gz)
		gz.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip payload: %w", err)
		}
	} else {
		pbPayload = bodyBytes[5:]
	}

	reqStruct := &workflowservice.StartWorkflowExecutionRequest{}
	if err := proto.Unmarshal(pbPayload, reqStruct); err != nil {
		return nil, fmt.Errorf("failed to unmarshal StartWorkflowExecutionRequest: %w", err)
	}

	fmt.Printf("Resolving workflowID: %s, namespaceID: %s\n", reqStruct.WorkflowId, reqStruct.Namespace)

	payload := &Payload{
		WorkflowID:       reqStruct.WorkflowId,
		VirtualNamespace: reqStruct.Namespace,
	}

	physNs, cacheHit := resolver.Resolve(payload, registry)
	fmt.Printf("Resolved virtual namespace '%s' to physical namespace '%s' (Cache hit: %v)\n", reqStruct.Namespace, physNs, cacheHit)

	ns := parsePhysicalNamespace(physNs)

	// Rewrite the target namespace to the physical namespace we resolved
	reqStruct.Namespace = ns.name

	// Dynamically route this request to the resolved cluster
	r.URL.Host = ns.cluster.address
	r.Host = ns.cluster.address

	// Re-marshal the updated protobuf payload
	newPbPayload, err := proto.Marshal(reqStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated StartWorkflowExecutionRequest: %w", err)
	}

	// We will send the payload uncompressed to avoid any gzip re-compression quirks.
	finalPayload := newPbPayload

	newBodyLen := len(finalPayload)
	newBody := make([]byte, 5+newBodyLen)
	// Set compression flag to 0 (uncompressed)
	newBody[0] = 0
	// Set the new message length in big-endian format
	newBody[1] = byte(newBodyLen >> 24)
	newBody[2] = byte(newBodyLen >> 16)
	newBody[3] = byte(newBodyLen >> 8)
	newBody[4] = byte(newBodyLen)

	copy(newBody[5:], finalPayload)

	// GRPc over HTTP/2 usually does not use Content-Length.
	// To be safe against length mismatch deadlocks, we delete it.
	r.ContentLength = -1
	r.Header.Del("Content-Length")

	return newBody, nil
}

func handlePollWorkflowTaskQueue(bodyBytes []byte) {

	if len(bodyBytes) <= 5 {
		fmt.Printf("Payload too short to be valid gRPC.\n")
		return
	}

	var pbPayload []byte
	isCompressed := bodyBytes[0] == 1
	if isCompressed {
		gz, err := gzip.NewReader(bytes.NewReader(bodyBytes[5:]))
		if err != nil {
			log.Printf("Failed to create gzip reader: %v\n", err)
			return
		}
		pbPayload, err = io.ReadAll(gz)
		gz.Close()
		if err != nil {
			log.Printf("Failed to decompress gzip payload: %v\n", err)
			return
		}
	} else {
		pbPayload = bodyBytes[5:]
	}

	reqStruct := &workflowservice.PollWorkflowTaskQueueRequest{}
	if err := proto.Unmarshal(pbPayload, reqStruct); err != nil {
		log.Printf("Failed to unmarshal PollWorkflowTaskQueueRequest: %v\n", err)
		return
	}

	fmt.Printf("PollWorkflowTaskQueue Request:\n")
	fmt.Printf("  Namespace: %s\n", reqStruct.Namespace)
}
