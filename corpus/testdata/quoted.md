<!-- @gloss documents zoo:Breeding scope=file -->
# Quoting the grammar

A comment may mention the grammar without being a marker: text between
backticks is quoted, and so is a fenced code block.

<!-- The literal `@gloss implements impl:Decoy` in backticks is prose, not a marker. -->
<!-- `Gloss-Implements: impl:Decoy` in backticks is prose too. -->

```python
# @gloss implements impl:Decoy
def decoy(): ...
```

~~~
<!-- @gloss represents impl:Decoy -->
~~~

<!-- @gloss documents zoo:Foaling -->
## Foaling

An unpaired backtick ` leaves the comment untouched.
