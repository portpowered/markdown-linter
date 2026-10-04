# Cache guide

## Problem statement

The cache stores entries, but entries can expire.

## Data

The table defines the input parameters.

| Name | Type | Description |
| --- | --- | --- |
| enabled | boolean | Enable the cache. |

```mermaid
flowchart LR
  accTitle: Cache flow
  accDescr: Requests enter the cache.
  A --> B
```

## Limitations

The cache has finite capacity.
