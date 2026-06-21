#!/usr/bin/env bash
temporal operator namespace delete --address=localhost:7233 payment  --yes
./add_namespace.sh default payment localhost:7233

temporal operator namespace delete --address=localhost:7233 pricing  --yes
./add_namespace.sh default pricing localhost:7233