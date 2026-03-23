# Methods

Methods are Go implementations of class functions available on native `python` types. For example:

```
{{ "hello".upper() }}
```

The following clickable admonition can be used to browse the `python` dedicated documentation for additional details:

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html) |
| ------------------------------------------------------------- |


## The `bool` type      

_Booleans are subtypes of integers in `python`._

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#additional-methods-on-integer-types) |
| ------------------------------------------------------------------------------------------------- |

### The `bit_length()` method

Returns the number of bits necessary to represent an integer in binary, excluding the sign and leading zeros.

### The `bit_count()` method

Returns the number of ones in the binary representation of the absolute value of the integer. This is also known as the population count.

## The `int` type      

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#additional-methods-on-integer-types) |
| ------------------------------------------------------------------------------------------------- |

### The `is_integer()` method

Returns `True`. Exists for duck type compatibility with `float.is_integer()`.


## The `float` type      

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#additional-methods-on-float) |
| ----------------------------------------------------------------------------------------- |

### The `is_integer()` method

Returns `True` if the float instance is finite with integral value, and `False` otherwise.

## The `str` type

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#string-methods) |
| ---------------------------------------------------------------------------- |

Most methods are implemented; Except for these:
- [maketrans](https://docs.python.org/3/library/stdtypes.html#str.maketrans) - complex
- [translate](https://docs.python.org/3/library/stdtypes.html#str.translate) - complex
- [isidentifier](https://docs.python.org/3/library/stdtypes.html#str.isidentifier) - not too relevant outside python.
- [index](https://docs.python.org/3/library/stdtypes.html#str.index) - have find without errors.
- [rindex](https://docs.python.org/3/library/stdtypes.html#str.rindex) - Have rfind without errors


We strive for 100% python compatibility but due to limited resources we are bound by the differences in golang and python. 
For example in go `unicode.IsNumber("三")` returns false for chinese numbers "三" but in python `!char.isnumeric()` returns true.

### The `capitalize()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.capitalize) |
| ------------------------------------------------------------- |

Return a copy of the string with its first character capitalized and the rest lowercased.

### The `capwords(sep=None)` method

| [🐍 `python`](https://docs.python.org/3/library/string.html#string.capwords) |
| ------------------------------------------------------------- |

Splits the argument into words, capitalizes each word, and joins them back as a single string. If the optional second argument sep is absent or None, several whitespace characters are replaced by a single space and leading and trailing whitespace characters are removed ; otherwise the `sep` is used to split and join the words.

### The `casefold()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.casefold) |
| ------------------------------------------------------------- |

Return a casefolded copy of the string. Casefolded strings may be used for caseless matching.

Casefolding is similar to lowercasing but more aggressive because it is intended to remove all case distinctions in a string. For example, the German lowercase letter 'ß' is equivalent to "ss". Since it is already lowercase, lower() would do nothing to 'ß'; casefold() converts it to "ss".

### The `center(width[, fillchar])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.center) |
| ------------------------------------------------------------- |

Returns a copy of the string of length `width` with the initial content being center. Padding is done using the specified `fillchar` (default is an ASCII space). The original string is returned if width is less than or equal to len(s).

### The `count(sub[, start[, end]])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.count) |
| ------------------------------------------------------------- |

Returns the number of non-overlapping occurrences of substring sub in the range `[start, end]`. Optional arguments `start` and `end` are interpreted as in slice notation.

If `sub` is empty, it returns the number of empty strings between characters which is the length of the string plus one.

### The `encode(encoding='utf-8', errors='strict')` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.encode) |
| ------------------------------------------------------------- |

Returns the string encoded to bytes. Encoding defaults to `'utf-8'` and only `'iso-8859-1'` is also supported. `errors` controls how encoding errors are handled and can be set to `'strict'` or `'ignore'`.

### The `endswith(suffix[, start[, end]])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.endswith) |
| ------------------------------------------------------------- |

Returns `True` if the string ends with the specified suffix, otherwise return `False`. `suffix` can also be a tuple of suffixes to look for. With the optional `start`, testing begins at that position. With the optional `end`, comparing stops at that position.

### The `expandtabs(tabsize=8)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.expandtabs) |
| ------------------------------------------------------------- |

Returns a copy of the string where all tab characters are replaced by one or more spaces, depending on the current column and the given tab size. Tab positions occur every `tabsize` characters (default is 8, giving tab positions at columns 0, 8, 16 and so on). To expand the string, the current column is set to zero and the string is examined character by character. If the character is a tab (`\t`), one or more space characters are inserted in the result until the current column is equal to the next tab position. (The tab character itself is not copied.) If the character is a newline (`\n`) or return (`\r`), it is copied and the current column is reset to zero. Any other character is copied unchanged and the current column is incremented by one regardless of how the character is represented when printed.

### The `find(sub[, start[, end]])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.find) |
| ------------------------------------------------------------- |

Returns the lowest index in the string where substring `sub` is found within the slice `[start:end]`. Optional arguments `start` and `end` are interpreted as in slice notation. Returns `-1` if `sub` is not found.

### The `format(*args, **kwargs)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.format) |
| ------------------------------------------------------------- |

Performs a string formatting operation. The string on which this method is called can contain literal text or replacement fields delimited by braces `{}`. Each replacement field contains either the numeric index of a positional argument, or the name of a keyword argument. Returns a copy of the string where each replacement field is replaced with the string value of the corresponding argument.

See format [string syntax](https://docs.python.org/3/library/string.html#formatstrings) for a description of the various formatting options that can be specified in format strings.

There are differences in python versions. We try to capture this with ["dialects" and default to `3.11`](https://github.com/NikolaLohinski/gonja/blob/master/builtins/methods/pystring/dialect.go). Override the DefaultDialect to get the desired behavior. 


### The `format_map(mapping)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.format_map) |
| ------------------------------------------------------------- |

Aliased to format since the difference in golang is not relevant.

### The `isalnum()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.isalnum) |
| ------------------------------------------------------------- |

Returns `True` if all characters in the string are alphanumeric and there is at least one character, `False` otherwise. A character `c` is alphanumeric if one of the following returns `True`: `c.isalpha()`, `c.isdecimal()`, `c.isdigit()`, or `c.isnumeric()`.

### The `isalpha()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.isalpha) |
| ------------------------------------------------------------- |

Returns `True` if all characters in the string are alphabetic and there is at least one character, `False` otherwise. Alphabetic characters are those characters defined in the Unicode character database as “Letter”, i.e., those with general category property being one of “Lm”, “Lt”, “Lu”, “Ll”, or “Lo”. Note that this is different from the Alphabetic property defined in the section 4.10 ‘Letters, Alphabetic, and Ideographic’ of the Unicode Standard.

### The `isascii()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.isascii) |
| ------------------------------------------------------------- |

Returns `True` if the string is empty or all characters in the string are ASCII, `False` otherwise. ASCII characters have code points in the range `U+0000-U+007F`.

### The `isdecimal()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.isdecimal) |
| ------------------------------------------------------------- |

Returns `True` if all characters in the string are decimal characters and there is at least one character, `False` otherwise. Decimal characters are those that can be used to form numbers in base 10, e.g. `U+0660`, `ARABIC-INDIC` `DIGIT` `ZERO`. Formally a decimal character is a character in the Unicode General Category “Nd”.

### The `isdigit()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.isdigit) |
| ------------------------------------------------------------- |

Returns `True` if all characters in the string are digits and there is at least one character, `False` otherwise. Digits include decimal characters and digits that need special handling, such as the compatibility superscript digits. This covers digits which cannot be used to form numbers in base 10, like the Kharosthi numbers. Formally, a digit is a character that has the property value `Numeric_Type=Digit` or `Numeric_Type=Decimal`.

### The `islower()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.islower) |
| ------------------------------------------------------------- |

Returns `True` if all cased characters in the string are lowercase and there is at least one cased character, `False` otherwise.

### The `isnumeric()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.isnumeric) |
| ------------------------------------------------------------- |

Returns True if all characters in the string are numeric characters, and there is at least one character, `False` otherwise. Numeric characters include digit characters, and all characters that have the Unicode numeric value property, e.g. `U+2155`, `VULGAR FRACTION ONE FIFTH`. Formally, numeric characters are those with the property value `Numeric_Type=Digit`,` Numeric_Type=Decimal` or `Numeric_Type=Numeric`.

### The `isprintable()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.isprintable) |
| ------------------------------------------------------------- |

Returns `True` if all characters in the string are printable or the string is empty, `False` otherwise. Non-printable characters are those characters defined in the Unicode character database as “Other” or “Separator”, excepting the ASCII space (0x20) which is considered printable (note that printable characters in this context are those which should not be escaped when `repr()` is invoked on a string. It has no bearing on the handling of strings written to `sys.stdout` or `sys.stderr`).

### The `isspace()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.isspace) |
| ------------------------------------------------------------- |

Returns `True` if there are only whitespace characters in the string and there is at least one character, `False` otherwise.

A character is whitespace if in the Unicode character database (see unicodedata), either its general category is Zs (“Separator, space”), or its bidirectional class is one of WS, B, or S.

### The `istitle()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.istitle) |
| ------------------------------------------------------------- |

Returns `True` if the string is a title-cased string and there is at least one character, for example uppercase characters may only follow uncased characters and lowercase characters only cased ones. Returns `False` otherwise.

### The `isupper()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.isupper) |
| ------------------------------------------------------------- |

Returns `True` if all cased characters in the string are uppercase and there is at least one cased character, `False` otherwise.

### The `join(iterable)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.join) |
| ------------------------------------------------------------- |

Returns a string which is the concatenation of the strings in `iterable`. An error will be raised if there are any non-string values in iterable, including bytes objects. The separator between elements is the string providing this method.

### The `ljust(width[, fillchar])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.ljust) |
| ------------------------------------------------------------- |

Returns the string left justified in a string of length `width`. Padding is done using the specified `fillchar` (default is an ASCII space). The original string is returned if width is less than or equal to len(s).

### The `lower()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.lower) |
| ------------------------------------------------------------- |

Returns a copy of the string with all the cased characters converted to lowercase.

The lowercasing algorithm used is described in section 3.13 "Default Case Folding" of the Unicode Standard.

### The `lstrip([chars])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.lstrip) |
| ------------------------------------------------------------- |

Returns a copy of the string with leading characters removed. The `chars` argument is a string specifying the set of characters to be removed. If omitted or None, the chars argument defaults to removing whitespace. The `chars` argument is not a prefix; rather, all combinations of its values are stripped:

### The `partition(sep)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.partition) |
| ------------------------------------------------------------- |

Splits the string at the first occurrence of `sep`, and returns a 3-tuple containing the part before the separator, the separator itself, and the part after the separator. If the separator is not found, return a 3-tuple containing the string itself, followed by two empty strings.

### The `removeprefix(prefix, /)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.removeprefix) |
| ------------------------------------------------------------- |

If the string starts with the `prefix` string, return `string[len(prefix):]`. Otherwise, returns a copy of the original string.

### The `removesuffix(suffix, /)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.removesuffix) |
| ------------------------------------------------------------- |

If the string ends with the `suffix` string and that `suffix` is not empty, returns `string[:-len(suffix)]`. Otherwise, returns a copy of the original string:

### The `replace(old, new[, count])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.replace) |
| ------------------------------------------------------------- |

Returns a copy of the string with all occurrences of substring `old` replaced by `new`. If the optional argument `count` is given, only the first `count` occurrences are replaced.

### The `rfind(sub[, start[, end]])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.rfind) |
| ------------------------------------------------------------- |

Returns the highest index in the string where substring `sub` is found, such that `sub` is contained within `string[start:end]`. Optional arguments `start` and `end` are interpreted as in slice notation. Return `-1` on failure.

### The `rjust(width[, fillchar])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.rjust) |
| ------------------------------------------------------------- |

Returns the string right justified in a string of length `width`. Padding is done using the specified `fillchar` (default is an ASCII space). The original string is returned if width is less than or equal to len(s).

### The `rpartition(sep)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.rpartition) |
| ------------------------------------------------------------- |

Splits the string at the last occurrence of `sep`, and return a 3-tuple containing the part before the separator, the separator itself, and the part after the separator. If the separator is not found, return a 3-tuple containing two empty strings, followed by the string itself.

### The `rsplit(sep=None, maxsplit=-1)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.rsplit) |
| ------------------------------------------------------------- |

Returns a list of the words in the string, using `sep` as the delimiter string. If `maxsplit` is given, at most `maxsplit` splits are done, the rightmost ones. If `sep` is not specified or `None`, any whitespace string is a separator. Except for splitting from the right, `rsplit()` behaves like `split()` which is described in detail below.

### The `rstrip([chars])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.rstrip) |
| ------------------------------------------------------------- |

Returns a copy of the string with trailing characters removed. The `chars` argument is a string specifying the set of characters to be removed. If omitted or `None`, the `chars` argument defaults to removing whitespace. The `chars` argument is not a suffix; rather, all combinations of its values are stripped

### The `split(sep=None, maxsplit=-1)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.split) |
| ------------------------------------------------------------- |

Returns a list of the words in the string, using `sep` as the delimiter string. If `maxsplit` is given, at most `maxsplit` splits are done (thus, the list will have at most `maxsplit+1` elements). If `maxsplit` is not specified or `-1`, then there is no limit on the number of splits (all possible splits are made).

If `sep` is given, consecutive delimiters are not grouped together and are deemed to delimit empty strings (for example, `'1,,2'.split(',')` returns `['1', '', '2']`). The `sep` argument may consist of multiple characters as a single delimiter (to split with multiple delimiters, use `re.split()`). Splitting an empty string with a specified separator returns `['']`.

### The `splitlines(keepends=False)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.splitlines) |
| ------------------------------------------------------------- |

Returns a list of the lines in the string, breaking at line boundaries. Line breaks are not included in the resulting list unless `keepends` is given and true.

This method splits on the following line boundaries. In particular, the boundaries are a superset of universal newlines.

### The `startswith(prefix[, start[, end]])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.startswith) |
| ------------------------------------------------------------- |

Returns `True` if the string starts with the prefix, otherwise return `False`. `prefix` can also be a tuple of prefixes to look for. With optional `start`, test string beginning at that position. With optional `end`, stop comparing string at that position.

### The `strip([chars])` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.strip) |
| ------------------------------------------------------------- |

Returns a copy of the string with the leading and trailing characters removed. The `chars` argument is a string specifying the set of characters to be removed. If omitted or `None`, the `chars` argument defaults to removing whitespace. The `chars` argument is not a prefix or suffix; rather, all combinations of its values are stripped

### The `swapcase()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.swapcase) |
| ------------------------------------------------------------- |

Returns a copy of the string with uppercase characters converted to lowercase and vice versa. Note that it is not necessarily true that the string verifies `string.swapcase().swapcase() == string`.

### The `title()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.title) |
| ------------------------------------------------------------- |

Returns a title-cased version of the string where words start with an uppercase character and the remaining characters are lowercase.

### The `upper()` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.upper) |
| ------------------------------------------------------------- |

Returns a copy of the string with all the cased characters converted to uppercase. Note that `string.upper().isupper()` might be `False` if the string contains uncased characters or if the Unicode category of the resulting character(s) is not “Lu” (Letter, uppercase), but e.g. “Lt” (Letter, titlecase).

The upper-casing algorithm used is described in section 3.13 ‘Default Case Folding’ of the Unicode Standard.

### The `zfill(width)` method

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#str.zfill) |
| ------------------------------------------------------------- |

Returns a copy of the string left filled with ASCII `'0'` digits to make a string of length width. A leading sign prefix (`'+'`/`'-'`) is handled by inserting the padding after the sign character rather than before. The original string is returned if width is less than or equal to len(s).

## The `list` type      

| [🐍 `python`](https://docs.python.org/3/tutorial/datastructures.html#data-structures) |
| ------------------------------------------------------------------------------------ |

### The `reverse()` method

Reverses the elements of the list in place.

### The `append(x)` method

Adds an item to the end of the list.

### The `copy()` method

Returns a shallow copy of the list.

## The `dict` type      

| [🐍 `python`](https://docs.python.org/3/library/stdtypes.html#mapping-types-dict) |
| -------------------------------------------------------------------------------- |

### The `keys()` method

Returns a list of the dictionary’s keys.
