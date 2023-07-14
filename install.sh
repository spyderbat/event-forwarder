#!/bin/bash -e

# WARNING: do not use /usr, /usr/local, or any other system directory.
# Don't change this without understanding the consequences and modifying
# the unit file.
INSTALL_HOME=/opt/spyderbat-events

ARCH=$(arch)
BIN=spyderbat-event-forwarder

if [ "$(whoami)" != "root" ] ; then
  echo "error: this needs to run as root"
  exit 1
fi

mkdir -p ${INSTALL_HOME}/bin
chmod 700 ${INSTALL_HOME}
mkdir -p ${INSTALL_HOME}/etc
mkdir -p ${INSTALL_HOME}/var/log

# Create the sbevents user/group if it does not exist
id -u sbevents >/dev/null 2>/dev/null ||
  useradd --system --home-dir ${INSTALL_HOME} --shell /bin/false sbevents

# Fix directory ownership
chgrp sbevents ${INSTALL_HOME}
chmod 750 ${INSTALL_HOME}
chown -R sbevents ${INSTALL_HOME}/var/log

# Install the program and systemd .service file
install -m755 ${BIN}.${ARCH} ${INSTALL_HOME}/bin/${BIN}
install -m644 ${BIN}.service /lib/systemd/system/${BIN}.service

# Install the config file if one doesn't already exist
if [ ! -e "${INSTALL_HOME}/etc/config.yaml" ] ; then
  install -m640 example_config.yaml ${INSTALL_HOME}/etc/config.yaml
fi

chgrp sbevents ${INSTALL_HOME}/etc/config.yaml
systemctl enable ${BIN}.service
systemctl daemon-reload

cat <<EOF

${BIN} is installed!

!!!!!!
Please edit the config file now:

    ${INSTALL_HOME}/etc/config.yaml
!!!!!!

To start the service, run:

    sudo systemctl start ${BIN}.service

To view the service status, run:

    sudo journalctl -fu ${BIN}.service

EOF
