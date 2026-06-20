#!/bin/bash

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <virtual_namespace>"
    echo "Example: $0 default"
    exit 1
fi

VIRTUAL_NAMESPACE=$1

curl -s -X GET "http://localhost:8089/namespace/virtual/get?name=${VIRTUAL_NAMESPACE}"
echo ""
