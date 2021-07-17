#!/bin/bash
/usr/sbin/dsconf localhost backend create --suffix dc=directory,dc=fedoraproject,dc=com --be-name firstUserRoot
echo -e '\nbasedn = dc=directory,dc=fedoraproject,dc=com' >> /data/config/container.inf
/usr/sbin/dsidm localhost initialise