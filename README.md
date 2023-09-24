# Peers Protocol Aggregator/Accumulator
implemention of HAProxy’s Peers Protocol(v2.1)

## Problems this solves
This code can be used to synchronize stick tables from multiple instances of HAProxy when operating in an active-active mode(agg mode) and to accumulate HAProxy's stick tables entries for accounting(currently, just addition) purposes(acc mode)

## Configuration

All configuration is through environment variables
The following table lists the configurable parameters and their default values.

Parameter | Description | Type | Default
--- | --- | --- | ---
`SERVICE_TCP_HOST` | the DOMAIN name or IP address used by tcp clients(peers) | `string` | `localhost`
`SERVICE_WEB_HOST` | the DOMAIN name or IP address used by http clients | `string` | `localhost`
`SERVICE_TCP_PORT` | the TCP port used by tcp clients(peers) | `string` | `11111`
`SERVICE_WEB_PORT` | the HTTP port used by http clients | `string` | `8060`
`SERVICE_NAME` | service name | `string` | `aggr1`
`SERVICE_MODE` | mode of operation (agg/acc) | `string` | `agg`
`SERVICE_AUTO_SYNC` | Enable service auto-sync mode (not required for haproxy) | `string` | `false`

## API

Path | Description
--- | ---
`/tables` | Retrieve the current values from the service tables

## HAProxy Config
```
peers mypeers
  bind 0.0.0.0:55555
  server haproxy1
  server aggr1 127.0.0.1:11111 # (server SERVICE_NAME SERVICE_TCP_HOST:SERVICE_TCP_PORT)
```

## Example
### synchronize stick tables (agg mode)
```
backend bk_test
        mode http
        balance roundrobin

        http-request set-header Room %[urlp(room)]
        stick-table type string len 128 size 2k expire 60m peers mypeers
        stick on hdr(Room)
        server server1 127.0.0.1:8080
        server server2 127.0.0.1:8881
        server server3 127.0.0.1:8882
```