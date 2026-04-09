# TODO

- Evaluate a colocated control-plane service model where a lightweight web service runs next to the Redis instance and executes expensive workspace operations locally to Redis, while the CLI becomes a remote client. Goal: reduce client-to-Redis round trips for operations like import, mount preparation, checkpoint creation, and restore on remote databases.
