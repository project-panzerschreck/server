# RPC Tracker

The RPC Tracker is a service that keeps track of RPC servers.
[The tracker is inspired by the Bittorent Tracker protocol.](https://www.bittorrent.org/beps/bep_0003.html#trackers)

## API

### Announce

To be added to the cluster, a client must announce itself to the tracker by sending a GET request to `/announce`.
The response will be a JSON object with the following fields:

- `interval`: The number of seconds the client should wait between announces.