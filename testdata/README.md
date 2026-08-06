# testdata

Test material that is read from disk rather than written in source.

`testdata/fixtures/` is declared binary in `.gitattributes`. Everything under it
is stored byte for byte: no line-ending conversion in either direction, and no
diff rendered as text. That is the point of the directory. A fixture that proves
what happens to a document with unusual line endings, an unusual encoding, or a
filename that was not written on this operating system is worthless if git
repairs it on the way in.

A fixture whose bytes are not the thing under test does not belong here. Put it
beside the test that reads it, where it is text like everything else and a
reviewer can read the diff.

The rule for a fixture whose bytes matter and that is small enough to inline is
in [CONTRIBUTING.md](../CONTRIBUTING.md): encode it in source rather than
committing a raw literal. A base64 string in a test file survives every
normalisation any tool applies, because the tool sees ASCII and the test decodes
it back to the bytes the case is about.
