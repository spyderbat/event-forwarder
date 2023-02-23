## This is a simple helm chart to install the Spyderbat Event Forwarder to a Kubernetes cluster where it will spit out events to stdout as well as a pvc backed file for easier consumption.
<br />

# Quickstart
```
git clone https://github.com/spyderbat/event-forwarder.git
cd event-forwarder/helm-chart/event-forwarder
helm install <release-name> . --namespace spyderbat --set spyderbat.spyderbat_org_uid=<ORG_ID> --set spyderbat.spyderbat_secret_api_key=<API_KEY> --create-namespace
```
<br/>

# Values to override

| value | description | default|required|
|--------|-------------|--------|----|
|spyderbat.spyderbat_org_uid | org uid to use | your_org_uid| Y|
|spyderbat.spyderbat_secret_api_key | api key from console | your_api_key|Y|
|spyderbat.api_host | api host to use | api.prod.spyderbat.com|N
|namespace| namespace to install to| spyderbat|N
|spyderbat.matching_filters | only write out events that match these regex filters (json/yaml array of strings syntax)|.*|N
|spyderbat.linkback | attempt to append a linkback to the spyderbat console for easy access |false |N 
|spyderbat.ui_url| ui url to use to generate linkback | "https://app.spyderbat.com" | N


<br />

# Validating install
### Run the following command
```
kubectl logs statefulset.apps/sb-forwarder-event-forwarder -n spyderbat
```
### You should see something like the below at the top of the logs followed by any/all events in your org (possibly filtered if using matching filters)
```
 tarting spyderbat-event-forwarder (commit unknown; unknown; go1.18.10; amd64)
loading config from ./config.yaml
org uid: spyderbatuid
api host: api.kangaroobat.net
log path: /opt/local/spyderbat/var/log
local syslog forwarding: false
{"id":"event_alert:k75NGuJ9Sn0:Y_fKWg:3259:iptables"...
```
