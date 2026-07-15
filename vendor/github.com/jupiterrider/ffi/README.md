# ffi
[![Go Reference](https://pkg.go.dev/badge/github.com/jupiterrider/ffi.svg)](https://pkg.go.dev/github.com/jupiterrider/ffi)

A purego binding for libffi.

## Purpose
You can use [purego](https://github.com/ebitengine/purego) to call C code without cgo. ffi provides extra functionality (e.g. passing and returning structs by value).

## Supported OS/Architecture
- darwin/amd64
- darwin/arm64
- freebsd/amd64
- freebsd/arm64
- linux/amd64
- linux/arm64
- linux/riscv64
- windows/amd64
- windows/arm64

## Software Requirements
[libffi](https://github.com/libffi/libffi) is preinstalled on most distributions, because it also is a dependency of Python and Ruby. If not, you can install it explicitly:

### Arch Linux
```sh
sudo pacman -S libffi
```

### Debian, Ubuntu
```sh
sudo apt install libffi8
```

### Fedora
```sh
sudo dnf install libffi
```

### FreeBSD
```sh
pkg install libffi
```
Note: Use this `-gcflags="github.com/ebitengine/purego/internal/fakecgo=-std"` build flag when cross compiling or having CGO_ENABLED set to 0 (FreeBSD only).

### Windows
The AMD64 version of libffi is already embedded into this library and gets extracted and loaded at runtime. This feature can be disabled by using the build tag `ffi_no_embed` or the environment variable `FFI_NO_EMBED=1`.

### macOS
No further requirements. The libffi binaries are embedded as well.

## Examples
In this example we create our own library, which consists of two type definitions and one function:

```c
#include <stdbool.h>
#include <string.h>

typedef enum {
    GROCERIES,
    HOUSEHOLD,
    BEAUTY
} Category;

typedef struct {
    const char *name;
    double price;
    Category category;
} Item;

bool IsItemValid(Item item)
{
    if (!item.name || strlen(item.name) == 0)
    {
        return false;
    }

    if (item.price < 0)
    {
        return false;
    }

    if (item.category > BEAUTY)
    {
        return false;
    }

    return true;
}
```

Compile the code into a shared library:

```sh
gcc -shared -o libitem.so -fPIC item.c
```

The consuming Go code:

```golang
package main

import (
	"fmt"

	"github.com/jupiterrider/ffi"
)

type Category uint32

const (
	Groceries Category = iota
	Household
	Beauty
)

type Item struct {
	Name     *byte
	Price    float64
	Category Category
}

func main() {
	// load the library
	lib, err := ffi.Load("./libitem.so")
	if err != nil {
		panic(err)
	}

	// create a new ffi.Type which defines the fields of the Item struct
	typeItem := ffi.NewType(&ffi.TypePointer, &ffi.TypeDouble, &ffi.TypeUint32)

	// get the IsItemValid function and describe its signature
	// (for bool we use ffi.TypeUint8)
	isItemValid, err := lib.Prep("IsItemValid", &ffi.TypeUint8, &typeItem)
	if err != nil {
		panic(err)
	}

	var item Item
	// strings are null-terminated and converted into a byte pointer
	item.Name = &[]byte("Apple\x00")[0]
	item.Price = 0.22
	item.Category = Groceries

	// the return value is stored in a 64-bit integer type, because libffi
	// cannot handle smaller integer types as return value
	var result ffi.Arg

	// call the C function
	// (keep in mind that you have to pass pointers and not the values themselves)
	isItemValid.Call(&result, &item)

	if result.Bool() {
		fmt.Println("Item is valid!")
	} else {
		fmt.Println("Item is not valid!")
	}
}
```

You can find more examples inside the [examples](examples) folder of this repository.
