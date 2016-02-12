# Style and conventions

## Traditionally hex settings should use JSON integers, not JSON strings

For example, [`"classID": 1048577`][class-id] instead of `"classID": "0x100001"`.
The config JSON isn't enough of a UI to be worth jumping through string <-> integer hoops to support an 0x… form ([source][integer-over-hex]).

## Constant names should keep redundant prefixes

For example, `CAP_KILL` instead of `KILL` in [**`linux.capabilities`**][capabilities]).
The redundancy reduction from removing the namespacing prefix is not useful enough to be worth trimming the upstream identifier ([source][keep-prefix]).

## Optional settings should have pointer Go types

So we have a consistent way to identify unset values ([source][optional-pointer]).

[capabilities]: config-linux.md#capabilities
[class-id]: config-linux.md#network
[integer-over-hex]: https://github.com/opencontainers/specs/pull/267#discussion_r48360013
[keep-prefix]: https://github.com/opencontainers/specs/pull/159#issuecomment-138728337
[optional-pointer]: https://github.com/opencontainers/specs/pull/233#discussion_r47829711
