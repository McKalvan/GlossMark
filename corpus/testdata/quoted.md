<!-- @gloss documents lib:Cataloging scope=file -->
# Quoting the grammar

A comment may mention the grammar without being a marker: text between
backticks is quoted, and so is a fenced code block.

<!-- The literal `@gloss implements circ:Decoy` in backticks is prose, not a marker. -->
<!-- `Gloss-Implements: circ:Decoy` in backticks is prose too. -->

```python
# @gloss implements circ:Decoy
def decoy(): ...
```

~~~
<!-- @gloss represents circ:Decoy -->
~~~

<!-- @gloss documents lib:Return -->
## Return

An unpaired backtick ` leaves the comment untouched.
