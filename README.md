# 389 Directory Server Input Plugin

This plugin gathers metrics from 389 Directory Servers's cn=Monitor backend and cn=monitor,cn=${database},cn=ldbm database,cn=plugins,cn=config for each indexes and database files.

### History

This EXECD Shim started out as a normal plugin merge request by:

  * [falon](https://github.com/falon) - OG Author
  * [gnulux](https://github.com/gnulux) - Contributer

Original Pull Request: https://github.com/influxdata/telegraf/pull/6117

### Contributers

  * [falon](https://github.com/falon)
  * [gnulux](https://github.com/gnulux)
  * [wisebaldone](https://github.com/wisebaldone)

### Features

- [X] Connect to 389DS server
  - [X] LDAP ( With StartTLS )
  - [X] LDAPS
  - [X] LDAPI
- [X] Basic Stats
- [X] LDBM Stats
  - [X] Per Database
- [ ] Connections

### Deploying

#### Docker ( Recommended )

A telegraf + this execd shim is published to ()

To build your own custom container with multiple plugins then see Dockerfile for an example. See 
docker-compose.yml for an example for a local environment when running 389ds in a container.

#### Manual

A static binary can be produced and deployed for a locally installed telegraf. To do this run:

```shell
go build -o telegraf-execd-389ds-db-input cmd/main.go
```

### Developing

When developing locally please follow the below steps:

1. docker-compose up --build
2. < wait for 389ds instance to provision >
3. docker exec -it ds "/bin/sh -c 'bootstap-389ds.sh'"

### Configuration:

```toml
[[inputs.ds389_db]]
  # Connection URL to 389DS Server
  # ldap / ldaps / ldapi supported
  url = "ldap://localhost:389"

  # Enable starttls, not needed for ldaps/ldapi
  starttls = true

  # skip peer certificate verification. Default is false.
  insecure_skip_verify = false

  # Path to PEM-encoded Root certificate to use to verify server certificate
  tls_ca = "/etc/ssl/certs.pem"

  # dn/password to bind with. If bind_dn is empty, an anonymous bind is performed.
  bind_dn = ""
  bind_password = ""
  
  ## Gather dbname to monitor
  # Comma separated list of db filename
  # dbtomonitor = ["exampleDB"]
  # If true, alldbmonitor monitors all db and overrides "dbtomonitor".
  alldbmonitor = false

  # Collects individual fields for connections.
  expand_connections = true
```

### Measurements & Fields:

All attributes are gathered based on this LDAP query:

`(objectClass=extensibleObject)`

Metric names are attributes name. 

If dbtomonitor array is provided , it can gather metrics for each dbfilename like uniquemember, memberof, givename indexes.
If `alldbmonitor = true`, `dbtomonitor` will be overriden with all dbs currently installed in the Directory Server.

An 389DS 2.x.x server will provide these metrics:

- ds389
  - tags:
    - url
- fields:
  - currentconnections
  - totalconnections
  - currentconnectionsatmaxthreads
  - maxthreadsperconnhits
  - dtablesize
  - readwaiters
  - opsinitiated
  - opscompleted
  - entriessent
  - bytessent
  - anonymousbinds
  - unauthbinds
  - simpleauthbinds
  - strongauthbinds
  - bindsecurityerrors
  - inops
  - readops
  - compareops
  - addentryops
  - removeentryops
  - modifyentryops
  - modifyrdnops
  - listops
  - searchops
  - onelevelsearchops
  - wholesubtreesearchops
  - referrals
  - chainings
  - securityerrors
  - errors
  - connections
  - connectionseq
  - connectionsinmaxthreads
  - connectionsmaxthreadscount
  - bytesrecv
  - bytessent
  - entriesreturned
  - referralsreturned
  - masterentries
  - copyentries
  - cacheentries
  - cachehits
  - slavehits

If you enable the `expand_connection` a full connection status detail will be added to the metrics.

### Expanded Connections

If `expand_connections = true` the `connection` attribute will be parsed in this metric:

    - the metric prefix is "conn."
    - the file dscriptor is then added to the metric name
    - the name of the monitored parameter is then concatenated with a dot

Format:

`conn.<fd>.<param> = <value>`

### Tags:

- url= # value from config

### Example Output:

