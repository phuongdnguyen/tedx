#!/bin/bash

if [ "$#" -ne 3 ]; then
    echo "Usage: $0 <virtual_namespace> <physical_namespace> <cluster_address>"
    echo "Example: $0 default default-3 localhost:7235"
    exit 1
fi

VIRTUAL_NAMESPACE=$1
PHYSICAL_NAMESPACE=$2
CLUSTER_ADDRESS=$3

curl -s -X POST http://localhost:8089/namespace/physical/remove \
  -H "Content-Type: application/json" \
  -d '{
    "virtual_namespace": "'"$VIRTUAL_NAMESPACE"'",
    "physical_namespace": "'"$PHYSICAL_NAMESPACE"'",
    "cluster_address": "'"$CLUSTER_ADDRESS"'"
  }'
echo ""
