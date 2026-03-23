# Tests

A test can be used in blocks and/or expressions to trigger conditional behavior, for example:

```
{% if variable is string %}
   This was a string: {{ variable }}
{% elif variable is sequence %}
   This was a list: {{ variable | join(",") }}
{% end if%}
```

The following clickable admonition can be used to browse the `python` dedicated documentation for additional details:

| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#list-of-builtin-tests) |
| ----------------------------------------------------------------------------------------- |

## The `callable` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.callable) |
| ---------------------------------------------------------------------------------------- |

Return whether the object is callable (i.e., some kind of function).

## The `defined` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.defined) |
| --------------------------------------------------------------------------------------- |

Tells whether a variable is defined.

## The  `undefined` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.undefined) |
| ----------------------------------------------------------------------------------------- |

Tells when a variable is not defined.

## The `divisibleby` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.divisibleby) |
| ------------------------------------------------------------------------------------------- |

Check if a variable is divisible by a number.
```
{% if 2048 is divisibleby 512 %}
    Yes it is modulo 4
{% endif %}
```

## The `eq`, `equalto` or `==` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.eq) |
| ---------------------------------------------------------------------------------- |

Classic equality comparisons.

## The `ne`  or `!=` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.ne) |
| ---------------------------------------------------------------------------------- |

Classic arithmetic inequality comparisons.

## The `ge` or `>=` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.ge) |
| ---------------------------------------------------------------------------------- |

Classic arithmetic comparisons.

## The `gt` or `>` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.gt) |
| ---------------------------------------------------------------------------------- |

Classic arithmetic comparisons.

## The `le` or `<=` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.le) |
| ---------------------------------------------------------------------------------- |

Classic arithmetic comparisons.


## The `lt` or `<` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.lt) |
| ---------------------------------------------------------------------------------- |

Classic arithmetic comparisons.

## The `even` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.even) |
| ------------------------------------------------------------------------------------ |

Tells whether a given number can be divided by 2.

## The `odd` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.odd) |
| ----------------------------------------------------------------------------------- |

Tells whether a given number can not be divided by 2.

## The `in` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.in) |
| ---------------------------------------------------------------------------------- |

Return whether the input contains the argument:
* on strings, tells whether the provided substring is part of the tested one ;
* on lists, tells whether the argument in the tested list ;
* on dictionaries, tells whether the argument is a key of the dictionary.
```
{{ "foo" is in "foobar" }}            // True
{{ 4 is in [1, 2, 3] }}               // False
{{ "key" is in {"key": "value"} }}    // True
{{ "value" is in {"key": "value"} }}  // False
```

## The `iterable` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.iterable) |
| ---------------------------------------------------------------------------------------- |

Check if it’s possible to iterate over the tested input, i.e the object is either a list, a dictionary or a string.

## The `none` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.none) |
| ------------------------------------------------------------------------------------ |

Return `True` if the input is `nil` or `None`

## The `mapping` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.mapping) |
| --------------------------------------------------------------------------------------- |

Classic type casting tests.

## The `sequence` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.sequence) |
| ---------------------------------------------------------------------------------------- |

Classic type casting tests.

## The `number` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.number) |
| -------------------------------------------------------------------------------------- |

Classic type casting tests.

## The `string` test
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-tests.string) |
| -------------------------------------------------------------------------------------- |

Classic type casting tests.