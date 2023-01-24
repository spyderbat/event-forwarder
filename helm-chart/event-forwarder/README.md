# This is a simple helm chart to install the Spyderbat Event Forwarder to a Kubernetes cluster where it will spit out events to stdout as well as a pvc backed file for easier consumption.

## Quickstart
```
helm install <release-name> . --namespace spyderbat --set spyderbat.spyderbat_org_uid=<ORG_ID> --set spyderbat.spyderbat_secret_api_key=<API_KEY> --create-namespace
```

## Values to override

| value | description | default|required|
|--------|-------------|--------|----|
|spyderbat.spyderbat_org_uid | org uid to use | your_org_uid| Y|
|spyderbat.spyderbat_secret_api_key | api key from console | your_api_key|Y|
|spyderbat.api_host | api host to use | api.prod.spyderbat.com|N
|namespace| namespace to install to| spyderbat|N
|persistence.storageClass | pvc storageClass | default|N