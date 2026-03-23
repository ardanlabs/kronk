# Filters

Variables can be modified by filters. Filters are separated from the variable by a pipe symbol (`|`) and may have optional arguments in parentheses. Multiple filters can be chained. The output of one filter is applied to the next.

```
{{ "a,b,c" | split(",") | tojson }}

{% set number = "123" | int %}

{% for x in {"first": 1, "second": 2} | values %}
  {{- x }}
{% endfor %}
```

The following clickable admonition can be used to browse the `python` dedicated documentation for additional details on each filter:

| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#builtin-filters) |
| ----------------------------------------------------------------------------------- |

## The `abs` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.abs) |
| ------------------------------------------------------------------------------------- |

Return the absolute value of the integer or float passed.

## The `attr` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.attr) |
| -------------------------------------------------------------------------------------- |

Get an attribute of an object. However, items are not looked up.

## The `batch` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.batch) |
| --------------------------------------------------------------------------------------- |

A filter that batches items. It returns a list of lists with the given number of items and will fill missing items if the second parameter `fille_with` is passed:
```html
<table>
{%- for row in items|batch(3, '&nbsp;') %}
  <tr>
  {%- for column in row %}
    <td>{{ column }}</td>
  {%- endfor %}
  </tr>
{%- endfor %}
</table>
```

## The `capitalize` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.capitalize) |
| -------------------------------------------------------------------------------------------- |

Capitalize a value. The first character will be uppercase, all others lowercase.

## The `center` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.center) |
| ---------------------------------------------------------------------------------------- |

Centers the value in a field of a given width.

## The `default` or `d` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.default) |
| ----------------------------------------------------------------------------------------- |

If the value is undefined it will return the passed default value, otherwise the value of the variable:
```
{{ my_variable | d('my_variable is not defined') }}
```

## The `dictsort` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.dictsort) |
| ------------------------------------------------------------------------------------------ |

Sort a dict and yield (key, value) pairs. Dictionaries may not be in the order you want to display them in, so sort them first.

## The `escape` or `e` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.escape) |
| ---------------------------------------------------------------------------------------- |

Replace the characters &, <, >, ', and " in the string with HTML-safe sequences. Use this if you need to display text that might contain such characters in HTML.

## The `filesizeformat` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.filesizeformat) |
| ------------------------------------------------------------------------------------------------ |

Format the value like a ‘human-readable’ file size (i.e. 13 kB, 4.1 MB, 102 Bytes, etc).

## The `first` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.first) |
| --------------------------------------------------------------------------------------- |

Return the first item of a sequence.

## The `float` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.float) |
| --------------------------------------------------------------------------------------- |

Convert the value into a floating point number

## The `forceescape` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.forceescape) |
| --------------------------------------------------------------------------------------------- |

Enforce HTML escaping. This will probably double escape variables.

## The `format` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.format) |
| ---------------------------------------------------------------------------------------- |

Apply the given values to a printf-style format string, like string % values.
```
{{ "%s, %s!"|format(greeting, name) }}
Hello, World!
```

## The `groupby` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.groupby) |
| ----------------------------------------------------------------------------------------- |

Group a sequence of objects by an attribute.

For example, a list of User objects with a city attribute can be rendered in groups. In this example, grouper refers to the city value of the group.

```html
<ul>{% for city, items in users | groupby("city") %}
  <li>{{ city }}
    <ul>{% for user in items %}
      <li>{{ user.name }}
    {% endfor %}</ul>
  </li>
{% endfor %}</ul>
```

## The `indent` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.indent) |
| ---------------------------------------------------------------------------------------- |

Return a copy of the string with each line indented by 4 spaces. The first line and blank lines are not indented by default.

## The `int` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.int) |
| ------------------------------------------------------------------------------------- |

Convert the value into an integer.

## The `join` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.join) |
| -------------------------------------------------------------------------------------- |

Return a string which is the concatenation of the strings in the sequence. The separator between elements is an empty string per default,

## The `last` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.last) |
| -------------------------------------------------------------------------------------- |

Return the last item of a sequence.

## The `length` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.length) |
| ---------------------------------------------------------------------------------------- |

Return the number of items in a container.

## The `list` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.list) |
| -------------------------------------------------------------------------------------- |

Convert the value into a list. If it was a string the returned list will be a list of characters.

## The `lower` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.lower) |
| --------------------------------------------------------------------------------------- |

Convert a value to lowercase.

## The `map` filter

| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.map) |
| ------------------------------------------------------------------------------------- |

Applies a filter on a sequence of objects or looks up an attribute. This is useful when dealing with lists of objects but you are really only interested in a certain value of it.

The basic usage is mapping on an attribute. Imagine you have a list of users but you are only interested in a list of usernames:

```
Users on this page: {{ users | map(attribute='username') | join(', ') }}
```

## The `max` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.max) |
| ------------------------------------------------------------------------------------- |

Return the largest item from the sequence.

## The `min` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.min) |
| ------------------------------------------------------------------------------------- |

Return the smallest item from the sequence.

## The `pprint` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.pprint) |
| ---------------------------------------------------------------------------------------- |

Pretty print a variable.

## The `random` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.random) |
| ---------------------------------------------------------------------------------------- |

Return a random item from the sequence.

## The `rejectattr` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.rejectattr) |
| -------------------------------------------------------------------------------------------- |

Filters a sequence of objects by applying a test to the specified attribute of each object, and rejecting the objects with the test succeeding.

If no test is specified, the attribute’s value will be evaluated as a boolean.

```
{{ users | rejectattr("is_active") }}
{{ users | rejectattr("email", "none") }}
```

## The `reject` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.reject) |
| ---------------------------------------------------------------------------------------- |

Filters a sequence of objects by applying a test to each object, and rejecting the objects with the test succeeding.

If no test is specified, each object will be evaluated as a boolean.

Example usage:

```
{{ numbers|reject("odd") }}
```

## The `replace` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.replace) |
| ----------------------------------------------------------------------------------------- |

Return a copy of the value with all occurrences of a substring replaced with a new one.

## The `reverse` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.reverse) |
| ----------------------------------------------------------------------------------------- |

Reverse the object or return an iterator that iterates over it the other way round.

## The `round` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.round) |
| --------------------------------------------------------------------------------------- |

Round the number to a given precision. The first parameter specifies the precision (default is 0), the second the rounding method:

* `common` rounds either up or down
* `ceil` always rounds up
* `floor` always rounds down

If you don’t specify a method `common` is used.

## The `safe` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.safe) |
| -------------------------------------------------------------------------------------- |

Mark the value as safe which means that in an environment with automatic escaping enabled this variable will not be escaped.

## The `selectattr` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.selectattr) |
| -------------------------------------------------------------------------------------------- |

Filters a sequence of objects by applying a test to the specified attribute of each object, and only selecting the objects with the test succeeding.

If no test is specified, the attribute’s value will be evaluated as a boolean.

```
{{ users | selectattr("is_active") }}
{{ users | selectattr("email", "none") }}
```

## The `select` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.select) |
| ---------------------------------------------------------------------------------------- |

Filters a sequence of objects by applying a test to each object, and only selecting the objects with the test succeeding.

If no test is specified, each object will be evaluated as a boolean.

```
{{ numbers | select("odd") }}
{{ numbers | select("odd") }}
{{ numbers | select("divisibleby", 3) }}
{{ numbers | select("lessthan", 42) }}
{{ strings | select("equalto", "mystring") }}
```

## The `slice` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.slice) |
| --------------------------------------------------------------------------------------- |

Slice an iterator and return a list of lists containing those items. Useful if you want to create a div containing three ul tags that represent columns:

```
<div class="columnwrapper">
  {%- for column in items|slice(3) %}
    <ul class="column-{{ loop.index }}">
    {%- for item in column %}
      <li>{{ item }}</li>
    {%- endfor %}
    </ul>
  {%- endfor %}
</div>
```

If you pass it a second argument it’s used to fill missing values on the last iteration.

## The `sort` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.sort) |
| -------------------------------------------------------------------------------------- |

Sort an iterable input.

## The `string` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.string) |
| ---------------------------------------------------------------------------------------- |

Convert an object to a string if it isn’t already.

## The `striptags` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.striptags) |
| ------------------------------------------------------------------------------------------- |

Strip SGML/XML tags and replace adjacent whitespace by one space.

## The `sum` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.sum) |
| ------------------------------------------------------------------------------------- |

Returns the sum of a sequence of numbers plus the value of parameter `start` (which defaults to 0). When the sequence is empty it returns `start`.

It is also possible to sum up only certain attributes:

```
Total: {{ items | sum(attribute='price') }}
```

## The `title` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.title) |
| --------------------------------------------------------------------------------------- |

Return a titlecased version of the value. I.e. words will start with uppercase letters, all remaining characters are lowercase.

## The `tojson` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.tojson) |
| ---------------------------------------------------------------------------------------- |

Serialize an object to a string of JSON. It takes an `indent` parameter to do pretty printing.

## The `trim` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.trim) |
| -------------------------------------------------------------------------------------- |

Strip leading and trailing characters, by default whitespace.

## The `truncate` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.truncate) |
| ------------------------------------------------------------------------------------------ |

Return a truncated copy of the string. The length is specified with the first parameter which defaults to 255.

## The `unique` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.unique) |
| ---------------------------------------------------------------------------------------- |

Returns a list of unique items from the given iterable.

```
{{ ['foo', 'bar', 'foobar', 'FooBar'] | unique }}
```
Will render:
```
['foo', 'bar', 'foobar']
```

Parameters:
* case_sensitive (default: false): Treat upper and lower case strings as distinct.
* attribute (default: None): Filter objects with unique values for this attribute.

## The `upper` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.upper) |
| --------------------------------------------------------------------------------------- |

Convert a value to uppercase.

## The `urlencode` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.urlencode) |
| ------------------------------------------------------------------------------------------- |

Quote data for use in a URL path or query using UTF-8.

## The `urlize` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.urlize) |
| ---------------------------------------------------------------------------------------- |

Convert URLs in text into clickable links.

## The `wordcount` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.wordcount) |
| ------------------------------------------------------------------------------------------- |

Count the words in that string.

## The `wordwrap` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.wordwrap) |
| ------------------------------------------------------------------------------------------ |

Wrap a string to the given width. Existing newlines are treated as paragraphs to be wrapped separately.

## The `xmlattr` filter
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#jinja-filters.xmlattr) |
| ----------------------------------------------------------------------------------------- |

Create an SGML/XML attribute string based on the items in a dict.