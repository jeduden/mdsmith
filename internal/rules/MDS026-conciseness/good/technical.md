# Technical Summary

Shard leaders persist commit indices, reject stale lease epochs,
and replicate snapshots across regions before applying writes.
