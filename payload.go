package main

import "math/rand/v2"

type Payload struct {
	WorkflowID       string
	VirtualNamespace string
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func GenerateRandomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		// rand.N selects a random index from the charset safely and efficiently
		b[i] = charset[rand.N(len(charset))]
	}
	return string(b)
}

func makePayloads(size int, vNamespace string) []*Payload {
	i := 0
	payloads := make([]*Payload, size)
	for i < size {
		payloads[i] = makePayload(vNamespace)
		i++
	}
	return payloads
}

func makePayload(vNamespace string) *Payload {
	return &Payload{
		WorkflowID:       GenerateRandomString(128),
		VirtualNamespace: vNamespace,
	}
}
