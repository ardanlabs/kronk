# PyString

An attempt to get as similar behavior as possible that exists in python.

Source reference: https://docs.python.org/3/library/string.html#string.Formatter

## Dialects
Occasionally, Python implementations may vary between versions necessitating
specification of the Python version to achieve direct parity. The aim is to
outline required feature flags necessary to attain compatibility in "dialects".

## Out of scope
Python 2.X `(string % dict)` compatibility. 3.X is enough.

## TODO
Format() - Support locale aware formatting
- [x] The 'z' option coerces negative zero floating-point values to positive zero after rounding to the format precision. This option is only valid for floating-point presentation types.
- [x] The ',' option signals the use of a comma for a thousands separator. For a locale aware separator, use the 'n' integer presentation type instead
- [x] The '_' option signals the use of an underscore for a thousands separator for floating point presentation types and for integer presentation type 'd'. For integer presentation types 'b', 'o', 'x', and 'X', underscores will be inserted every 4 digits. For other presentation types, specifying this option is an error.

Other features
- [ ] Support Template strings https://docs.python.org/3/library/string.html#template-strings

Str Functions
- [x] [capitalize](https://docs.python.org/3/library/stdtypes.html#str.capitalize)
- [x] [capwords](https://docs.python.org/3/library/string.html#string.capwords) - static strings utility
- [x] [casefold](https://docs.python.org/3/library/stdtypes.html#str.casefold)
- [x] [center](https://docs.python.org/3/library/stdtypes.html#str.center)
- [x] [count](https://docs.python.org/3/library/stdtypes.html#str.count)
- [x] [encode](https://docs.python.org/3/library/stdtypes.html#str.encode)
- [x] [endswith](https://docs.python.org/3/library/stdtypes.html#str.endswith)
- [x] [expandtabs](https://docs.python.org/3/library/stdtypes.html#str.expandtabs)
- [x] [find](https://docs.python.org/3/library/stdtypes.html#str.find)
- [x] [format](https://docs.python.org/3/library/stdtypes.html#str.format)
- [x] [format_map](https://docs.python.org/3/library/stdtypes.html#str.format_map) - Aliased to format since the difference in golang is not relevant
- [x] [isalnum](https://docs.python.org/3/library/stdtypes.html#str.isalnum)
- [x] [isalpha](https://docs.python.org/3/library/stdtypes.html#str.isalpha)
- [x] [isascii](https://docs.python.org/3/library/stdtypes.html#str.isascii)
- [x] [isdecimal](https://docs.python.org/3/library/stdtypes.html#str.isdecimal)
- [x] [isdigit](https://docs.python.org/3/library/stdtypes.html#str.isdigit)
- [x] [islower](https://docs.python.org/3/library/stdtypes.html#str.islower)
- [x] [isnumeric](https://docs.python.org/3/library/stdtypes.html#str.isnumeric)
- [x] [isprintable](https://docs.python.org/3/library/stdtypes.html#str.isprintable)
- [x] [isspace](https://docs.python.org/3/library/stdtypes.html#str.isspace)
- [x] [istitle](https://docs.python.org/3/library/stdtypes.html#str.istitle)
- [x] [isupper](https://docs.python.org/3/library/stdtypes.html#str.isupper)
- [x] [join](https://docs.python.org/3/library/stdtypes.html#str.join) - Implemented as JoinString() & JoinStringer()
- [x] [ljust](https://docs.python.org/3/library/stdtypes.html#str.ljust)
- [x] [lower](https://docs.python.org/3/library/stdtypes.html#str.lower)
- [x] [lstrip](https://docs.python.org/3/library/stdtypes.html#str.lstrip)
- [x] [partition](https://docs.python.org/3/library/stdtypes.html#str.partition)
- [x] [removeprefix](https://docs.python.org/3/library/stdtypes.html#str.removeprefix)
- [x] [removesuffix](https://docs.python.org/3/library/stdtypes.html#str.removesuffix)
- [x] [replace](https://docs.python.org/3/library/stdtypes.html#str.replace)
- [x] [rfind](https://docs.python.org/3/library/stdtypes.html#str.rfind)
- [x] [rjust](https://docs.python.org/3/library/stdtypes.html#str.rjust)
- [x] [rpartition](https://docs.python.org/3/library/stdtypes.html#str.rpartition)
- [x] [rsplit](https://docs.python.org/3/library/stdtypes.html#str.rsplit)
- [x] [rstrip](https://docs.python.org/3/library/stdtypes.html#str.rstrip)
- [x] [split](https://docs.python.org/3/library/stdtypes.html#str.split)
- [x] [splitlines](https://docs.python.org/3/library/stdtypes.html#str.splitlines)
- [s] [startswith](https://docs.python.org/3/library/stdtypes.html#str.startswith)
- [x] [strip](https://docs.python.org/3/library/stdtypes.html#str.strip)
- [x] [swapcase](https://docs.python.org/3/library/stdtypes.html#str.swapcase)
- [x] [title](https://docs.python.org/3/library/stdtypes.html#str.title)
- [x] [upper](https://docs.python.org/3/library/stdtypes.html#str.upper)
- [x] [zfill](https://docs.python.org/3/library/stdtypes.html#str.zfill)

Low Priority
- [] [maketrans](https://docs.python.org/3/library/stdtypes.html#str.maketrans)
- [] [translate](https://docs.python.org/3/library/stdtypes.html#str.translate)
- [] [isidentifier](https://docs.python.org/3/library/stdtypes.html#str.isidentifier) - not too relevant outside python.
- [] [index](https://docs.python.org/3/library/stdtypes.html#str.index) - have find without errors.
- [] [rindex](https://docs.python.org/3/library/stdtypes.html#str.rindex) - Have rfind without errors
