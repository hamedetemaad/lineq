# LineQ
implemention of Virtual Waiting Room Using HAProxy’s Peers Protocol(v2.1)

<p align="center">
  <img src="static/lineq.gif" width="700" title="lineq-vwr mode">
</p>

## What is Virtaul Waiting Room
Please read this article by <a href="https://blog.cloudflare.com/cloudflare-waiting-room/">Cloudflare.</a>

## Problems this solves
LineQ can be used to implement Virtual Waiting Room using HAProxy's stick tables(vwr mode) and synchronize stick tables from multiple instances of HAProxy when operating in an active-active mode(agg mode) or to accumulate HAProxy's stick tables entries for accounting(currently, just addition) purposes(acc mode)

## Configuration

All configuration is through environment variables
The following table lists the configurable parameters and their default values.

Parameter | Mode | Description | Type | Default
--- | --- | --- | --- | ---
`SERVICE_TCP_HOST` | general | the DOMAIN name or IP address used by tcp clients(peers) | `string` | `localhost`
`SERVICE_WEB_HOST` | general | the DOMAIN name or IP address used by http clients | `string` | `localhost`
`SERVICE_TCP_PORT` | general | the TCP port used by tcp clients(peers) | `string` | `11111`
`SERVICE_WEB_PORT` | general | the HTTP port used by http clients | `string` | `8060`
`SERVICE_MODE` | general | mode of operation (vwr/agg/acc) | `string` | `agg`
`SERVICE_VWR_SESSION_DURATION` | vwr | The time a visitor can remain idle on the web site (in minutes)?  | `string` | `1`
`SERVICE_VWR_TOTAL_USERS` | vwr | The number of visitors that can be on the website at the same time | `string` | `1`
`SERVICE_VWR_ROOM_TABLE` | vwr | related to stick tables | `string` | `room`
`SERVICE_VWR_USERS_TABLE` | vwr | related to stick tables | `string` | `timestamps`

## API

Path | Description
--- | ---
`/tables` | Retrieve the current values from the service tables


## Options

Option | Mode | Description
--- |--- | ---
`-c` | vwr | generation of basic haproxy configuration

## HAProxy Config
```
peers lineq
  bind 0.0.0.0:55555
  server haproxy1
  server lineq 127.0.0.1:11111 # (server SERVICE_NAME SERVICE_TCP_HOST:SERVICE_TCP_PORT)
```

## Example
### virtual waiting room (vwr mode)
see examples directory

### synchronize stick tables (agg mode)
```
backend bk_test
        mode http
        balance roundrobin

        http-request set-header Room %[urlp(room)]
        stick-table type string len 128 size 2k expire 60m peers lineq
        stick on hdr(Room)
        server server1 127.0.0.1:8080
        server server2 127.0.0.1:8881
        server server3 127.0.0.1:8882
```
