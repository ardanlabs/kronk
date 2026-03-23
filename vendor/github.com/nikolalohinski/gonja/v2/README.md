<div align="center">
<img src="docs/logo.png" />
<h1><code>gonja</code></h1>
</div>

`gonja` is a pure `go` implementation of the [Jinja template engine](https://jinja.palletsprojects.com/). It aims to be as compatible as possible with the original `python` implementation.

## Usage

### As a library

Install or update using `go get`:
```
go get github.com/nikolalohinski/gonja/v2
```

### As a `terraform` provider

This `gonja` library has been packaged as a `terraform` provider. For more information, please refer to the [dedicated documentation](https://registry.terraform.io/providers/NikolaLohinski/jinja/latest/docs).

## Example

```golang
package main

import (
	"os"
	"fmt"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
)

func main() {
	template, err := gonja.FromString("Hello {{ name | capitalize }}!")
	if err != nil {
		panic(err)
	}

	data := exec.NewContext(map[string]interface{}{
		"name": "bob",
	})
	
	if err = template.Execute(os.Stdout, data); err != nil { // Prints: Hello Bob!
		panic(err)
	}
}
```

## Documentation

* For details on how the **Jinja** template language works, please refer to [the Jinja documentation](https://jinja.palletsprojects.com) ;
* **gonja** API documentation is available on [godoc](https://godoc.org/github.com/nikolalohinski/gonja/v2) ;
* **filters**: please refer to [`docs/filters.md`](docs/filters.md) ;
* **control structures**: please take a look at [`docs/control_structures.md`](docs/control_structures.md) ;
* **tests**: please see [`docs/tests.md`](docs/tests.md) ;
* **global functions**: please browse through [`docs/global_functions.md`](docs/global_functions.md).
* **global variables**: please open [`docs/global_variables.md`](docs/global_variables.md).
* **methods**: please take a peek at [`docs/methods.md`](docs/methods.md).

## Migrating from `v1` to `v2`

As this project now aims to reproduce the behavior of the `python` Jinja engine as closely as possible, some backwards incompatible changes have been made from the initial draft and need to be taken into account when upgrading from `v1.X.X`. Moreover, please do note that `v1.X.X` versions are not maintained.

The following steps can be used as general guidelines to migrate from `v1` to `v2`:

* All references to `gonja` need to be changed from `"github.com/nikolalohinski/gonja"` to `"github.com/nikolalohinski/gonja/v2"`
* The following top level global variables/functions have been removed/updated and need to be adjusted accordingly:
	* `DefaultEnv` function is now called `DefaultEnvironment` and its properties have changed. See [gonja.go](./gonja.go) and [exec/environment.go](./exec/environment.go) for details
	* `FromCache` function has been removed as caching logic was removed. If required, it can be done by implementing a custom `Loader` (see [`loaders/loader.go`](./loaders/loader.go))
	* `Globals` is now referred to as `DefaultContext`
* What was called a `Statement` is now referred to as `ControlStructure` to be closer to `python`'s Jinja glossary and may require changes in consumer code
* What was called `Globals` is now called `GlobalFunctions` to be closer to `python`'s Jinja glossary and may require changes in consumer code
* All non-`python` built-ins have been removed from `gonja`. They have been moved to the [`terraform-provider-jinja` code base](https://github.com/NikolaLohinski/terraform-provider-jinja). They can be brought back as needed by adding the `github.com/NikolaLohinski/terraform-provider-jinja/lib` dependency, and updating the global variables defined in [`builtins/`](./builtins/) with the available methods for each (see [`exec/environment.go`](./exec/environment.go) for details)
* The `Execute` method of the `*exec.Template` object now requires a `io.Writer` to be passed, to be closer to Golang's `template` package interface. However, the `ExecuteToString` method now exists and behaves exactly as the `Execute` method used to, so it can be used as drop-in replacement.

## Limitations 

* **format**: `format` does **not** take `python`'s string format syntax as a parameter, instead it takes Go's. Essentially `{{ 3.14|stringformat:"pi is %.2f" }}` is `fmt.Sprintf("pi is %.2f", 3.14)`
* **escape** / **force_escape**: Unlike Jinja's behavior, the `escape`-filter is applied immediately. Therefore there is no need for a `force_escape` filter
* Only subsets of native `python` types (`bool`, `int`, `float`, `str`, `dict` and `list`) methods have been re-implemented in Go and can slightly differ from the original ones

## Development

### Guidelines

Please read through the [contribution guidelines](./CONTRIBUTING.md) before diving into any work.

### Requirements

- Install go `>= 1.21` by following the [official documentation](https://go.dev/doc/install) ;
- Install `ginkgo` by [any means you see fit](https://onsi.github.io/ginkgo/).

### Tests

The unit tests can be run using:

```sh
ginkgo run -p ./...
```

## Tribute

A massive thank you to the original author [@noirbizarre](https://github.com/noirbizarre) for doing the initial work in https://github.com/noirbizarre/gonja which this project was forked from.
