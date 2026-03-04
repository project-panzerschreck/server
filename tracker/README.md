# RPC Tracker

The RPC Tracker is a service that keeps track of RPC servers.
[The tracker is inspired by the Bittorent Tracker protocol.](https://www.bittorrent.org/beps/bep_0003.html#trackers)

## API

### Announce

To be added to the cluster, a client must announce itself to the tracker by sending a GET request to `/announce`.

Requests should contain the following query parameters:

- `port`: The port on which the client's RPC server is listening.
- `ip` (Optional): The IP address of the client's RPC server. This should generally be left unset, in which case the IP address of the request will be used.

The response will be a JSON object with the following fields:

- `interval`: The number of seconds the client should wait between announces.

Example Response:
```json
{
    "interval": 30
}
```

### List Servers

A list of the servers that have announced themselves to the tracker can be retreived from `/servers`.

The response will be a JSON object with the following fields:

- `servers`: A list of the servers that have announced themselves to the tracker. Each server is represented as a string of the form "ip:port".

Example Response:
```json
{
    "servers": ["192.168.1.123:6767", "192.168.1.42:6767"]
}
```