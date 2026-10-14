# Choosing how to edit a file

Two families of edit methods do different jobs. Pick the family first, then the method.

**On this page:** [Decision table](#decision-table) · [If X, use Y](#if-x-use-y) · [Turning a setting off](#turning-a-setting-off) · [Writing the result](#writing-the-result) · [Pitfalls](#pitfalls) · [Related](#related)

## Decision table

| Family | Methods | Semantics | Use for | Guide | Example |
|---|---|---|---|---|---|
| **Upsert** | `Set`, `SetAfter`, `SetBefore` | one active entry per key: an existing key is updated in place (its last occurrence), a new key is added | editing a file people maintain | [editing, steps 1-2](../library/editing.md#step-1--change-or-add-a-value-with-set) | [`ExampleFile_Set`](../../example_edit_test.go), [`ExampleFile_SetAfter`](../../example_edit_test.go) |
| **Literal** | `Append`, `InsertAfter`, `InsertBefore` with `NewPair`, `NewComment`, `NewBlank` | places exactly the entries you pass, duplicates included | generating a file or a section | [editing, steps 7-8](../library/editing.md#step-7--generate-content-with-the-constructors) | [`ExampleFile_Append`](../../example_edit_test.go), [`ExampleFile_InsertAfter`](../../example_edit_test.go) |

The difference in one line: on `A=1`, `Set("A", "2")` leaves one entry, `Append(NewPair("A", "2"))` leaves two, and the last one wins ([`ExampleFile_Append_vsSet`](../../example_edit_test.go)).

## If X, use Y

- **If you change a value that may or may not exist**, use `Set`. It returns `created=true` when it appended a new line. Setting the value a key already has is a true no-op: the bytes do not change ([`ExampleFile_Set_noop`](../../example_edit_test.go)).
- **If a new key belongs next to related keys**, use `SetAfter(anchor, key, value)` or `SetBefore`. A missing anchor returns `ErrAnchorNotFound` and changes nothing; an existing key is updated where it is and never moved ([`ExampleFile_SetAfter_anchorNotFound`](../../example_edit_test.go)).
- **If you generate content with comments and blank lines**, build entries with the constructors and place them with `Append` (end of file) or `InsertAfter` / `InsertBefore` (next to an anchor, as one block).
- **If you copy entries from another file**, pass entries from its `Entries()` to `Append` or `Insert*`: an attached entry is copied verbatim, byte for byte ([`ExampleFile_Append_verbatim`](../../example_edit_test.go)).
- **If you derive one environment from another**, use `Clone`, then `Set` what differs, then `SaveAs`. References stay references, so derived values follow your edit ([`ExampleFile_Clone`](../../example_edit_test.go)). Do not rebuild the file from `ExpandedMap()`: that bakes every reference ([`ExampleFile_Clone_baked`](../../example_edit_test.go)).
- **If you want to pin one value on purpose**, read it with `GetExpanded` and `Set` the result. It no longer follows its source.

## Turning a setting off

| Goal | Call | Result | Undo | Example |
|---|---|---|---|---|
| Turn it off, keep it in the file | `Unset(key, false)` | every line of the entry commented out | `Restore(key)`, byte-identical | [`ExampleFile_Unset`](../../example_edit_test.go), [`ExampleFile_Restore`](../../example_edit_test.go) |
| Remove it | `Unset(key, true)` | the entry is deleted | none; set it again | [`ExampleFile_Unset`](../../example_edit_test.go) |
| See what is turned off | `Disabled()` | every commented-out setting, as `Pair` values | — | [`ExampleFile_Disabled`](../../example_edit_test.go) |

Prefer `Unset(key, false)` for anything a person might want back: the comment above it stays attached, and the change is one line in a diff.

## Writing the result

| Call | Writes to | Mode | Guide | Example |
|---|---|---|---|---|
| `Save()` | `f.Path` | an existing file keeps its mode; a new file is `0600` | [saving, step 1](../library/saving.md#step-1--save-to-the-file-you-opened) | [`ExampleFile_Save`](../../example_edit_test.go), [`ExampleFile_Save_mode`](../../example_edit_test.go) |
| `SaveAs(path)` | `path`; `f.Path` is not changed | an existing destination keeps its mode; a new one gets the source file's mode | [saving, step 4](../library/saving.md#step-4--write-a-copy-with-saveas) | [`ExampleFile_SaveAs`](../../example_edit_test.go) |
| `Render()` | nothing; returns the bytes as a string | — | [saving](../library/saving.md#how-a-save-works) | [`ExampleParse`](../../example_read_test.go) |

Both save calls write a temp file at `0600` and rename it over the destination, so a reader never sees a half-written file.

## Pitfalls

- **`Set` on a key that is only disabled appends a new active line**; the commented-out one stays. To turn the original back on, call `Restore` instead.
- **`Set` re-renders the value.** It keeps the `export` prefix, indentation, spacing on both sides of the delimiter (`A = 1` becomes `A = 2`), the `=` or `:` delimiter and any inline comment, and it picks the quoting for the new value ([`ExampleFile_Set_quoting`](../../example_edit_test.go)).
- **`Set` stores a value literally.** `Set("P", "a$b")` writes `P='a$b'`, so `GetExpanded` gives `a$b` back, before and after a reload. The one exception is a value holding both a `'` and a backtick: no literal quote is left, so it is double-quoted and its `$` stays a reference ([`ExampleFile_Set_dollar`](../../example_edit_test.go)). In library v0.1.1 and earlier, the `$` was written bare and read back as a reference.
- **`Set` cannot write a reference.** `Set("URL", "${HOST}/v1")` stores the text `${HOST}/v1`, not a reference to `HOST`. To write a reference, `Append` a parsed line: `f.Append(dotenv.Parse("URL=${HOST}/v1\n").Entries()[0])` ([`ExampleFile_Append_verbatim`](../../example_edit_test.go)).
- **Duplicates: every method targets the last occurrence**, because it is the one a consumer sees. `Restore` also picks the last disabled entry, which may not be the one `Unset` just created.
- **Appending after an unterminated quote** puts the new lines inside that value. The source was already malformed ([`ExampleFile_Append_unterminatedQuote`](../../example_edit_test.go)).

## Related

- [Editing a .env file](../library/editing.md) · [Saving a .env file](../library/saving.md)
- [How to read a file](reading.md) · [Options for your kind of app](app-setups.md)
- [All choosing guides](README.md) · [Feature index](../reference/features.md)
