# This is a simple helm chart to install the Spyderbat Event Forwarder to a Kubernetes cluster where it will spit out events to stdout as well as a pvc backed file for easier consumption.

## Quickstart
```
helm install <release-name> . --set spyderbat.spyderbat_org_uid=<ORG_ID> --set spyderbat.spyderbat_secret_api_key=<API_KEY> --create-namespace
```