This is a reporter for the [go-metrics](https://github.com/rcrowley/go-metrics)
library which will post the metrics to Graphite. It was originally part of the
`go-metrics` library itself, but has been split off to make maintenance of
both the core library and the client easier.

### Usage

```go
import "github.com/cyberdelia/go-metrics-graphite"


go graphite.Graphite(metrics.DefaultRegistry,
  1*time.Second, "some.prefix", addr)
```

### Migrating from `rcrowley/go-metrics` implementation

Simply modify the import from `"github.com/rcrowley/go-metrics/librato"` to
`"github.com/cyberdelia/go-metrics-graphite"` and it should Just Work.
