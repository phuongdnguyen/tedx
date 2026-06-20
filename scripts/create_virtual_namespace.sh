#!/bin/bash

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <virtual_namespace>"
    echo "Example: $0 staging"
    exit 1
fi

VIRTUAL_NAMESPACE=$1

curl -s -X POST http://localhost:8089/namespace/virtual/create \
  -H "Content-Type: application/json" \
  -d '{
    "name": "'"$VIRTUAL_NAMESPACE"'"
  }'
echo ""
