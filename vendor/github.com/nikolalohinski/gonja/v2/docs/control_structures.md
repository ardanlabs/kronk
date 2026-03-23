# Control Structures

This section describes the syntax and semantics of the template engine and will be most useful as reference to those creating Jinja templates. A _control structure_  (or _statement_) is a special keyword that can be used through blocks in order to achieve conditional logic in a template.

The following clickable admonition can be used to browse the `python` dedicated documentation for additional details:

| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#list-of-control-structures) |
| ---------------------------------------------------------------------------------------------- |

## The `if` control structure
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#if) |
| ---------------------------------------------------------------------- |

The `if` control structure in Jinja is comparable with `python`'s `if` control structure.

```
{% if kenny.sick %}
    Kenny is sick.
{% elif kenny.dead %}
    You killed Kenny!  You bastard!!!
{% else %}
    Kenny looks okay --- so far
{% endif %}
```

## The `set` control structure
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#assignments) |
| ------------------------------------------------------------------------------- |

Inside code blocks, you can also assign values to variables:

```
{% set groceries = ["eggs", "milk", "vegetables"] %}
{% set csv = groceries | join(",") }
```

For more details on scoping especially within a `for` loop, please refer to the `python` [implementation documentation](https://jinja.palletsprojects.com/en/3.0.x/templates/#assignments).

## The `for` control structure
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#for) |
| ----------------------------------------------------------------------- |

Loop over each item in a sequence. For example, to display a list of users provided in a variable called users:

```html
<ul>
{% for user in [{"name": "bob", "name": "alice}] %}
  <li>{{ user.name }}</li>
{% endfor %}
</ul>
```

The `for` control structure can also iterate over dictionaries, and return a key/value pair:
```html
<ul>
{% for key, value in {"one": 1, "two": 2} %}
  <li>{{ key }} is represented as {{ value }}</li>
{% endfor %}
</ul>
```

For more details on the special variables available within the loop, please refer to the [dedicated `python` documentation](https://jinja.palletsprojects.com/en/3.0.x/templates/#list-of-control-structures)



## The `include` control structure
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#include) |
| --------------------------------------------------------------------------- |

The include tag is useful to include a template and return the rendered contents of that file into the current namespace:

```
{% include 'header.html' %}
    Body
{% include 'footer.html' %}
```

## The `with` control structure
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#with-statement) |
| ---------------------------------------------------------------------------------- |

The with control structure makes it possible to create a new inner scope. Variables set within this scope are not visible outside of the scope.

```
{% with foo = 42 %}
    {{ foo }}
{% endwith %}
```
Which is equivalent to:
```
{% with %}
    {% set foo = 42 %}
    {{ foo }}
{% endwith %}
```

## The `filter` control structure
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#id11) |
| ------------------------------------------------------------------------ |

Filter sections allow you to apply regular Jinja filters on a full node of template data. It just wraps the code in the special `filter` section:

```
{% filter upper %}
    This text becomes uppercase
{% endfilter %}
```


## The `raw` control structure
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#escaping) |
| ---------------------------------------------------------------------------- |

It is sometimes desirable – even necessary – to have Jinja ignore parts it would otherwise handle as variables or blocks and is possible with the `raw` control structure:
```html
{% raw %}
    <ul>
    {% for item in seq %}
        <li>{{ item }}</li>
    {% endfor %}
    </ul>
{% endraw %}
```

## The `block` and `extends` control structures
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#child-template) |
| ---------------------------------------------------------------------------------- |

The most powerful part of Jinja is template inheritance. Template inheritance allows you to build a base “skeleton” template that contains all the common elements of your site and defines blocks that child templates can override.

This template, which we’ll call `base.html`, defines a simple HTML skeleton document that you might use for a simple two-column page. It’s the job of “child” templates to fill the empty blocks with content:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    {% block head %}
    <link rel="stylesheet" href="style.css" />
    <title>{% block title %}{% endblock %} - My Webpage</title>
    {% endblock %}
</head>
<body>
    <div id="content">{% block content %}{% endblock %}</div>
    <div id="footer">
        {% block footer %}
        &copy; Copyright 2008 by <a href="http://domain.invalid/">you</a>.
        {% endblock %}
    </div>
</body>
</html>
```

A child template might look like this:

```html
{% extends "base.html" %}
{% block title %}Index{% endblock %}
{% block head %}
    {{ super() }}
    <style type="text/css">
        .important { color: #336699; }
    </style>
{% endblock %}
{% block content %}
    <h1>Index</h1>
    <p class="important">
      Welcome to my awesome homepage.
    </p>
{% endblock %}
```

The `{% extends %}` tag is the key here. It tells the template engine that this template “extends” another template. When the template system evaluates this template, it first locates the parent. The extends tag should be the first tag in the template. Everything before it is printed out normally and may cause confusion. Also a block will always be filled in regardless of whether the surrounding condition is evaluated to be `True` or `False`.

## The `import` and `macro` control structures
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#import) |
| -------------------------------------------------------------------------- |

Jinja supports putting often used code into macros. Macros are comparable with functions in regular programming languages. They are useful to put often used idioms into reusable functions to not repeat yourself (“DRY”). These macros can go into different templates and get imported from there. This works similarly to the `import` control structures in Python.


```html
{% macro input(name, value='', type='text', size=20) -%}
    <input type="{{ type }}" name="{{ name }}" value="{{
        value|e }}" size="{{ size }}">
{%- endmacro %}
```

The macro can then be called like a function in the namespace:

```html
<p>{{ input('username') }}</p>
<p>{{ input('password', type='password') }}</p>
```

To access another template’s variables and macros, you can `import` the whole template module into a variable. That way, you can access the attributes:

```html
{% import 'forms.html' as forms %}
<dl>
    <dt>Username</dt>
    <dd>{{ forms.input('username') }}</dd>
    <dt>Password</dt>
    <dd>{{ forms.input('password', type='password') }}</dd>
</dl>
<p>{{ forms.textarea('comment') }}</p>
```

Alternatively, you can `import` specific names from a template into the current namespace:
```html
{% from 'forms.html' import input as input_field, textarea %}
<dl>
    <dt>Username</dt>
    <dd>{{ input_field('username') }}</dd>
    <dt>Password</dt>
    <dd>{{ input_field('password', type='password') }}</dd>
</dl>
<p>{{ textarea('comment') }}</p>
```
Included templates have access to the variables of the active context by default.

## The `autoescape` control structure
| [🐍 `python`](https://jinja.palletsprojects.com/en/3.0.x/templates/#autoescape-overrides) |
| ---------------------------------------------------------------------------------------- |

If you want you can activate and deactivate the autoescaping from within the templates.
