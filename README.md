# Couchdb Replicator Setup

`couchdb-repl` is a utility to setup replication documents in multiple Couchdb database servers.
The setup is such that all servers replicate with all other servers.

## Usage

`user` - Set a username for accessing the servers. This must be the same on all servers.
`password` - Set a password for accessing the servers. This must be the same on all servers.
`server-url` - Set a URL of a server. Use this argument at least twice.
`db` - Set a name of a database to replicate. Use this argument at least once.

See [basic.hcl](examples/basic.hcl) for an example how to use this in combination with [J2](https://github.com/pulcy/j2).
