# Typed configuration

How to turn the string values of a `.env` into a typed Go struct (`int`, `bool`, `time.Duration`, slices), with the standard library or with a binder library.

**On this page**

- [Why every value is a string](#why-every-value-is-a-string)
- [Before you start](#before-you-start)
- [Step 1 — Get the expanded values](#step-1--get-the-expanded-values)
- [Step 2 — Bind them with the standard library](#step-2--bind-them-with-the-standard-library)
- [Step 3 — Or bind them with go-envconfig](#step-3--or-bind-them-with-go-envconfig)
- [Step 4 — Or bind them with caarlos0/env](#step-4--or-bind-them-with-caarlos0env)
- [Step 5 — Or use cfgkit for layered configuration](#step-5--or-use-cfgkit-for-layered-configuration)
- [Try it](#try-it)
- [Pitfalls](#pitfalls)
- [Related](#related)

## Why every value is a string

A `.env` has no types: `PORT=8443` is four characters. Which Go type it becomes depends on the field that receives it, which is your application's decision, so the library hands you strings and you bind them, the same way `encoding/json` unmarshals into a struct instead of offering `json.GetInt`. There is deliberately no `f.Int("PORT", 8080)`: a typed getter that returns its default for `PORT=eighty` brings the service up on the wrong port with nothing in the logs. A binder reports the bad value as an error, together with every other problem in the file.

Guessing types from a value's shape was rejected too: `ZIP=01234` would lose its leading zero, and `DESCRIPTION=Hello, world` would become a list.

## Before you start

```sh
go get github.com/ubgo/dotenv
```

Steps 3 to 5 also need the binder you choose (`go get github.com/sethvargo/go-envconfig`, `go get github.com/caarlos0/env/v11`, or `go get github.com/ubgo/cfgkit`). The library depends on none of them, and none of them depends on it.

## Step 1 — Get the expanded values

Bind the **expanded** values, so `${DOMAIN}` references, defaults and plugins have already been applied. From a path, `dotenv.Read(path)` returns them as a `map[string]string`. From a `*File` you already hold (from `Open` or `Parse`), call `f.ExpandedMap()`. Both return an error for an unsatisfied `${VAR:?message}` or a failing plugin, so check it before binding: that is how a file can mark a value required on its own, before any binder runs.

Pass the same options you would anywhere else. `dotenv.Read(".env", dotenv.WithLookup(os.LookupEnv))` lets the file reference process environment variables; see [plugins.md](plugins.md).

## Step 2 — Bind them with the standard library

You do not need a library for a handful of fields: `strconv` and `time.ParseDuration` do the conversion, and `errors.Join` reports every problem at once. Defaults apply only when a key is absent; a key that is present but malformed is an error.

<!-- snippet: example_plugin_test.go#plug_typedConfig -->
```go
// plugConfig is the typed view an application wants of its .env.
type plugConfig struct {
	Port  int
	Debug bool
	Wait  time.Duration
	Hosts []string
	Token string
}

// Defaults applied when a key is absent. A key that is present but malformed
// is an error, never a silent fall back to these.
const (
	plugDefaultPort = 8080
	plugDefaultWait = 30 * time.Second
	plugHostsSep    = ";" // HOSTS is a ;-separated list
)

// plugBind converts expanded string values into plugConfig. It collects every
// problem and returns them together, so one run shows everything to fix.
func plugBind(m map[string]string) (plugConfig, error) {
	cfg := plugConfig{Port: plugDefaultPort, Wait: plugDefaultWait}
	var errs []error

	if v, ok := m["PORT"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("PORT: %q is not an integer", v))
		}
		cfg.Port = n
	}
	if v, ok := m["DEBUG"]; ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("DEBUG: %q is not a boolean", v))
		}
		cfg.Debug = b
	}
	if v, ok := m["WAIT"]; ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("WAIT: %q is not a duration", v))
		}
		cfg.Wait = d
	}
	if v := m["HOSTS"]; v != "" {
		cfg.Hosts = strings.Split(v, plugHostsSep)
	}
	cfg.Token = m["TOKEN"]
	if cfg.Token == "" {
		errs = append(errs, errors.New("TOKEN: required"))
	}
	return cfg, errors.Join(errs...)
}

// Example_typedConfig binds expanded strings to a typed struct with the standard
// library only, reporting every malformed value at once.
func Example_typedConfig() {
	// dotenv supplies expanded strings; the application decides their types.
	good := "DOMAIN=acme.io\n" +
		"PORT=8443\n" +
		"DEBUG=true\n" +
		"HOSTS=api.${DOMAIN};www.${DOMAIN}\n" +
		"TOKEN=t-123\n"

	m, err := dotenv.Parse(good).ExpandedMap()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	cfg, err := plugBind(m)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%+v\n", cfg)

	// Every problem is reported at once, and none falls back to a default.
	bad := "PORT=eighty\nWAIT=soon\n"
	m, err = dotenv.Parse(bad).ExpandedMap()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	_, err = plugBind(m)
	fmt.Println(err)
	// Output:
	// {Port:8443 Debug:true Wait:30s Hosts:[api.acme.io www.acme.io] Token:t-123}
	// PORT: "eighty" is not an integer
	// WAIT: "soon" is not a duration
	// TOKEN: required
}
```

Example: [`Example_typedConfig`](../../example_plugin_test.go). In an application, replace `dotenv.Parse(good).ExpandedMap()` with `dotenv.Read(".env")`.

## Step 3 — Or bind them with go-envconfig

[`sethvargo/go-envconfig`](https://pkg.go.dev/github.com/sethvargo/go-envconfig) reads struct tags and takes its values from a `Lookuper`. Its `MapLookuper` wraps the map from Step 1, so expansion errors surface before binding:

<!-- snippet: exempt — third-party dependency; compiled against github.com/sethvargo/go-envconfig@v1.4.3 on 2026-10-14 -->
```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sethvargo/go-envconfig"
	"github.com/ubgo/dotenv"
)

type Config struct {
	Port  int           `env:"PORT, default=8080"`
	Debug bool          `env:"DEBUG"`
	Hosts []string      `env:"HOSTS, delimiter=;"`
	Wait  time.Duration `env:"WAIT, default=30s"`
	Token string        `env:"TOKEN, required"`
}

func main() {
	m, err := dotenv.Read(".env")
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	err = envconfig.ProcessWith(context.Background(), &envconfig.Config{
		Target:   &cfg,
		Lookuper: envconfig.MapLookuper(m),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", cfg)
}
```

With the `.env` from Step 2 (`DOMAIN`, `PORT=8443`, `DEBUG=true`, `HOSTS=api.${DOMAIN};www.${DOMAIN}`, `TOKEN=t-123`) and an empty process environment, it prints:

```text
{Port:8443 Debug:true Hosts:[api.acme.io www.acme.io] Wait:30s Token:t-123}
```

go-envconfig stops at the first field it cannot decode: with `PORT=eighty`, `WAIT=soon` and no `TOKEN`, the only error is `Port: strconv.ParseInt: parsing "eighty": invalid syntax`.

If you would rather resolve each key on demand from a `*File`, a three-line adapter satisfies go-envconfig's `Lookuper` interface (`Lookup(key string) (string, bool)`). It has to drop `GetExpanded`'s error, because that interface has no error return; see [Pitfalls](#pitfalls).

<!-- snippet: exempt — third-party dependency; compiled against github.com/sethvargo/go-envconfig@v1.4.3 on 2026-10-14 -->
```go
type fileLookuper struct{ f *dotenv.File }

func (l fileLookuper) Lookup(key string) (string, bool) {
	v, ok, err := l.f.GetExpanded(key)
	if err != nil {
		return "", false
	}
	return v, ok
}
```

Pass it as `Lookuper: fileLookuper{f}` in the `envconfig.Config` above. go-envconfig versions before v1.0 took the lookuper as a separate argument, `envconfig.ProcessWith(ctx, &cfg, lookuper)`.

## Step 4 — Or bind them with caarlos0/env

[`caarlos0/env`](https://pkg.go.dev/github.com/caarlos0/env/v11) reads struct tags and accepts a map of values through `Options.Environment`. When that map is set, it is used instead of the process environment.

<!-- snippet: exempt — third-party dependency; compiled against github.com/caarlos0/env/v11@v11.4.1 on 2026-10-14 -->
```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/ubgo/dotenv"
)

type Config struct {
	Port  int           `env:"PORT" envDefault:"8080"`
	Debug bool          `env:"DEBUG"`
	Hosts []string      `env:"HOSTS" envSeparator:";"`
	Wait  time.Duration `env:"WAIT" envDefault:"30s"`
	Token string        `env:"TOKEN,required"`
}

func main() {
	m, err := dotenv.Read(".env")
	if err != nil {
		log.Fatal(err)
	}

	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: m}); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", cfg)
}
```

With the same `.env` it prints the same `{Port:8443 Debug:true Hosts:[api.acme.io www.acme.io] Wait:30s Token:t-123}`. Unlike go-envconfig, caarlos0/env reports every binding error together. With `PORT=eighty`, `WAIT=soon` and no `TOKEN`:

```text
env: parse error on field "Port" of type "int": strconv.ParseInt: parsing "eighty": invalid syntax; parse error on field "Wait" of type "time.Duration": unable to parse duration: time: invalid duration "soon"; required environment variable "TOKEN" is not set
```

## Step 5 — Or use cfgkit for layered configuration

`github.com/ubgo/cfgkit` is a configuration system built on this library. It reads through `dotenv` rather than through a plain map, so quoting, multi-line values and `${VAR}` expansion (including `${VAR:-default}` and `${VAR:?message}`) behave exactly as described in these guides, and it uses the `Lookuper` seam so a later file in a `.env` / `.env.local` chain can reference a value an earlier one defined.

<!-- snippet: exempt — third-party dependency; compiled against github.com/ubgo/cfgkit@v0.1.0 on 2026-10-14 -->
```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ubgo/cfgkit"
)

type Config struct {
	Port  int           `env:"PORT" default:"8080"`
	Debug bool          `env:"DEBUG"`
	Hosts []string      `env:"HOSTS" delim:";"`
	Wait  time.Duration `env:"WAIT" default:"30s"`
	Token string        `env:"TOKEN,required" secret:"true"`
}

func main() {
	// DefaultSources reads the .env / .env.local chain from the working
	// directory, then the process environment. Load returns a *Config.
	cfg, res, err := cfgkit.Load[Config](cfgkit.DefaultSources())
	if err != nil {
		log.Fatal(err)
	}
	_ = res // provenance: which source set each field
	fmt.Printf("%+v\n", *cfg)
}
```

With the same `.env` it prints `{Port:8443 Debug:true Hosts:[api.acme.io www.acme.io] Wait:30s Token:t-123}`. Note the tag differences from the other two binders: defaults go in a separate `default:` tag and a list separator in `delim:`. cfgkit also reports every problem at once; with `PORT=eighty`, `WAIT=soon` and no `TOKEN`:

```text
cfgkit: 3 problem(s):
Port (PORT from file:.env,.env.local,.env.dev,.env.dev.local): "eighty" is not a valid int
Wait (WAIT from file:.env,.env.local,.env.dev,.env.dev.local): "soon" is not a valid duration
Token (TOKEN) is required but no source supplied it
```

On top of binding, it reports which source set each field (`res`), masks values marked secret, validates a configuration without booting the application, and generates the `.env.example` contract from the struct. See [cfgkit's documentation](https://github.com/ubgo/cfgkit) for its full tag list and options.

Which to pick: go-envconfig and caarlos0/env are small and do one thing; cfgkit is a layered configuration system; the standard-library binder in Step 2 needs no dependency at all. This library works with all of them.

## Try it

```sh
go test -run '^Example_typedConfig$' -v .
```

```text
=== RUN   Example_typedConfig
--- PASS: Example_typedConfig (0.00s)
PASS
ok  	github.com/ubgo/dotenv	0.005s
```

The example passes only when its output matches the `// Output:` block in Step 2.

## Pitfalls

- **Bind expanded values, not raw ones.** `f.Map()` and `f.Get` return values as written, so `HOSTS=api.${DOMAIN}` would reach the binder with the `${DOMAIN}` still in it. Use `Read`, `ExpandedMap` or `GetExpanded`.
- **The per-key adapter hides errors.** go-envconfig's `Lookuper` cannot return an error, so the `fileLookuper` adapter turns a `${VAR:?message}` failure or a secret-store error into "not set". A field with `default=` then silently takes its default. Call `f.ExpandedMap()` first, or use `MapLookuper` as in Step 3.
- **`Read` stops at the first expansion error.** A missing `${TOKEN:?…}` aborts the whole read with a `*dotenv.RequiredError` before any binder runs; you see that one error, not a list.
- **An empty value is present.** `PORT=` is in the map with value `""`. The Step 2 binder and cfgkit report it as not an integer; caarlos0/env v11 uses `envDefault` (8080); go-envconfig v1 sets the field to `0` with no error and without applying `default=`. Check how your binder treats empty values.
- **Disabled and inherited lines are not values.** `# PORT=8080` and a bare `PORT` line are not in the map. A name-only `PORT` declares that the value comes from the process environment, which the library never reads: apply `os.Getenv` yourself (see [entries.md](entries.md)).

## Related

- [plugins.md](plugins.md) — `WithLookup(os.LookupEnv)`, secret stores and decryption that run before binding.
- [expansion.md](expansion.md) — `${VAR:?message}` and the other operators that decide what each value expands to.
- [reading.md](reading.md) — `Read`, `Open`, `Parse` and `ExpandedMap`.
- [Feature index](../reference/features.md#library-plugins-and-hooks) — the typed-config row and its example.
- Example: [`Example_typedConfig`](../../example_plugin_test.go).
