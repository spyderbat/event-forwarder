# Panther Webhook Configuration: Getting Started

This is not meant to be a comprehensive guide to use the event forwarder with Panther.

## Panther schema configuration

Panther requires an ingestion schema to ingest log data. An example schema is provided [Here](Custom.SpyderbatR0.schema.yaml)

Download the example schema. In the Panther console, under Configure / Schemas, click
"Create New" and give the schema a name, such as SpyderbatR0.

Paste the contents of the example schema in the text box. Validate the schema, then save it.

## Panther log source configuration

Configure a log source in Panther. In the Panther console, under Configure / Log Sources, click
"Create New" and select "custom log formats."

Next, click "Start" under the category for HTTP logs.

Give the source a name, e.g. Spyderbat Forwarder on HOST_NAME (32 chars max)

Select the Custom.SpyderbatR0 schema created in the previous step.

Set the auth method to Bearer and click the refresh button to generate a bearer secret.
Then copy the secret.

*NOTE: Once you leave this screen, the secret cannot be retrieved again; It must be replaced.*

Click the "Setup" button.

The bearer secret must be converted to base64. An easy way to do this
with the Unix shell is to type:

```
echo -n YOUR_SECRET | base64
```

Keep this base64 secret handy for the webhook configuration step.

## Event forwarder configuration

Edit the `/opt/spyderbat-events/etc/config.yaml` configuration file.

### Configure your API key and org UUID

`spyderbat_org_uid` and `spyderbat_secret_api_key` must be valid. Note
that API keys are scoped to a user and not an org; It is recommended
to create a service user in the Spyderbat UI, grant it access to
the appropriate org, and generate the API key for the service user.
API keys expire after 1 year; Plan ahead to keep the key updated.

### Add a filter expression such as the one below to capture relevant data

```
expr: |
    schema startsWith "model_spydertrace:"
    and suppressed == false
    and (
        (score ?? 0) > 50
        or len(policy_name ?? "") > 0
    )
```

### Add a webhook configuration

Your panther source will have an HTTP Ingest URL associated with it. Retrieve it and the secret
you created earlier on, and add the webhook configuration:

```
webhook:
  endpoint_url: PANTHER_INGEST_URL
  compression_algo: zstd
  max_payload_bytes: 500000
  authentication:
    method: bearer
    parameters:
      secret_key: YOUR_BASE64_SECRET
```

Save config.yaml file and restart the event forwarder:

`sudo sytemctl restart spyderbat-event-forwarder.service`

Tail the logs to check for errors:

`sudo journalctl -fu spyderbat-event-forwarder.service`

