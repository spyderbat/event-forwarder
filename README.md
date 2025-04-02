# Spyderbat Event Forwarder

The event forwarder is a small utility that consumes Spyderbat events from the API and emits flat files containing events and spydertraces. It optionally forwards the data it collects via syslog or using a webhook.

## Features

- Deploys as a systemd service on Linux or into Kubernetes using Helm
- Consumes events and traces from the Spyderbat API
- Writes data to flat files and/or stdout
- Forwards events and traces via syslog or webhook (optional)

## Requirements

- Linux
- x86_64 or arm64 processor
- systemd

## Helm Installation
[Here](helm-chart/event-forwarder/README.md)

## Installation

Download the [latest release](https://github.com/spyderbat/event-forwarder/releases).

1. Unpack the tarball:

NOTE: The release package filename will differ from the example below.

```
mkdir /tmp/sef
tar xfz spyderbat-event-forwarder.5b41e00.tgz -C /tmp/sef
```

2. Run the installer:

```
cd /tmp/sef
sudo ./install.sh
```

You should see output like this:

```

spyderbat-event-forwarder is installed!

!!!!!!
Please edit the config file now:

    /opt/spyderbat-events/etc/config.yaml
!!!!!!

To start the service, run:

    sudo systemctl start spyderbat-event-forwarder.service

To view the service status, run:

    sudo journalctl -fu spyderbat-event-forwarder.service

```

3. Edit the config file:

`sudo vi /opt/spyderbat-events/etc/config.yaml`

4. Start the service:

`sudo systemctl start spyderbat-event-forwarder.service`

5. Check the service:

`sudo journalctl -fu spyderbat-event-forwarder.service`

Use ^C to interrupt the log. If you see errors, check the configuration, restart the service, and check again.

6. Enable the service to run at boot time:

`sudo systemctl enable spyderbat-event-forwarder.service`

7. If desired, integrate with the Splunk universal forwarder:

```
$ sudo splunk add monitor /opt/spyderbat-events/var/log/spyderbat_events.log
Your session is invalid.  Please login.
Splunk username: <your splunk username>
Password: <your splunk password>
Added monitor of '/opt/spyderbat-events/var/log/spyderbat_events.log'.
```
