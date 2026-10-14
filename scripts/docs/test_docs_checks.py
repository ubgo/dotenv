"""Self-tests for the docs checks: each check must pass good input and fail bad input.

Run: python3 -m unittest discover scripts/docs
Fixtures are built in a fresh temporary directory per test; no repo file is read or changed.
"""
import os
import sys
import tempfile
import textwrap
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import linkcheck  # imported after the sys.path line above
import snippets
import words
import wrapcheck

GO_FILE = textwrap.dedent('''\
    package demo_test

    import "fmt"

    // snippet:start demo_Hello
    func ExampleHello() {
    	fmt.Println("hello")
    	// Output: hello
    }

    // snippet:end demo_Hello

    func ExampleNoOutput() {
    	// snippet:start demo_NoOutput
    	fmt.Println("x")
    	// snippet:end demo_NoOutput
    }

    // snippet:start demo_helper
    func demoShow(s string) { fmt.Println(s) }

    // snippet:end demo_helper

    func ExampleShow() {
    	demoShow("y")
    	// Output: y
    }

    // snippet:start demo_orphan
    func demoUnused() {}

    // snippet:end demo_orphan
    ''')

HELLO_BLOCK = textwrap.dedent('''\
    func ExampleHello() {
    	fmt.Println("hello")
    	// Output: hello
    }
    ''')


def md_with(marker, body, lang="go", indent=""):
    lines = ["# Doc", ""]
    if marker is not None:
        lines.append(indent + marker)
    lines.append(indent + "```" + lang)
    lines += [(indent + l) if l else "" for l in body.rstrip("\n").split("\n")]
    lines += [indent + "```", ""]
    return "\n".join(lines)


class SnippetTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = self.tmp.name
        self.write("demo_test.go", GO_FILE)

    def tearDown(self):
        self.tmp.cleanup()

    def write(self, path, text):
        full = os.path.join(self.root, path)
        os.makedirs(os.path.dirname(full), exist_ok=True)
        with open(full, "w", encoding="utf-8") as fh:
            fh.write(text)

    def run_check(self, md_text, fix=False, go=("demo_test.go",)):
        self.write("doc.md", md_text)
        return snippets.run(self.root, ["doc.md"], list(go), fix=fix, out=lambda *_: None)

    def problems_for(self, md_text):
        _, _, probs = self.run_check(md_text)
        # The fixture deliberately holds two bad regions; keep only doc-level problems here.
        return [p for p in probs if p.startswith("doc.md")]

    def test_synced_block_passes(self):
        s, e, probs = self.run_check(md_with("<!-- snippet: demo_test.go#demo_Hello -->", HELLO_BLOCK))
        self.assertEqual((s, e), (1, 0))
        self.assertEqual([p for p in probs if p.startswith("doc.md")], [])

    def test_synced_block_inside_list_item_passes(self):
        text = "- step\n\n" + md_with("<!-- snippet: demo_test.go#demo_Hello -->", HELLO_BLOCK, indent="  ").split("\n", 2)[2]
        self.assertEqual(self.problems_for(text), [])

    def test_mismatched_block_fails(self):
        bad = HELLO_BLOCK.replace('"hello")', '"hullo")')
        probs = self.problems_for(md_with("<!-- snippet: demo_test.go#demo_Hello -->", bad))
        self.assertEqual(len(probs), 1)
        self.assertIn("differs", probs[0])

    def test_unmarked_go_block_fails(self):
        probs = self.problems_for(md_with(None, HELLO_BLOCK))
        self.assertEqual(len(probs), 1)
        self.assertIn("no snippet marker", probs[0])

    def test_marker_separated_by_blank_line_fails(self):
        text = "# Doc\n\n<!-- snippet: demo_test.go#demo_Hello -->\n\n```go\n" + HELLO_BLOCK + "```\n"
        probs = self.problems_for(text)
        self.assertTrue(any("no snippet marker" in p for p in probs), probs)
        self.assertTrue(any("not directly followed" in p for p in probs), probs)

    def test_exempt_needs_reason(self):
        self.assertEqual(self.problems_for(md_with("<!-- snippet: exempt — signature excerpt -->", "type X int")), [])
        probs = self.problems_for(md_with("<!-- snippet: exempt -->", "type X int"))
        self.assertEqual(len(probs), 1)
        self.assertIn("needs a reason", probs[0])

    def test_marker_before_non_go_block_fails(self):
        probs = self.problems_for(md_with("<!-- snippet: demo_test.go#demo_Hello -->", "echo hi", lang="sh"))
        self.assertEqual(len(probs), 1)

    def test_shell_block_without_marker_is_ignored(self):
        self.assertEqual(self.problems_for(md_with(None, "echo hi", lang="sh")), [])

    def test_missing_region_and_file_fail(self):
        self.assertIn("not found", self.problems_for(md_with("<!-- snippet: demo_test.go#nope -->", "x"))[0])
        self.assertIn("does not exist", self.problems_for(md_with("<!-- snippet: other_test.go#demo_Hello -->", "x"))[0])
        self.assertIn("not a *_test.go", self.problems_for(md_with("<!-- snippet: demo.go#demo_Hello -->", "x"))[0])

    def test_region_rules(self):
        _, _, probs = self.run_check("# empty\n")
        joined = "\n".join(probs)
        self.assertIn("ExampleNoOutput (region demo_NoOutput) has no // Output:", joined)
        self.assertIn("helper demoUnused in region demo_orphan is not called", joined)
        self.assertNotIn("demo_helper", joined)
        self.assertNotIn("demo_Hello", joined)
        self.assertEqual(len(probs), 2, probs)

    def test_region_names_unique_repo_wide(self):
        self.write("b/other_test.go", GO_FILE)
        _, _, probs = self.run_check("# empty\n", go=("demo_test.go", "b/other_test.go"))
        self.assertTrue(any("demo_Hello already used at demo_test.go" in p for p in probs), probs)

    def test_unclosed_region_fails(self):
        self.write("u_test.go", "package u_test\n\n// snippet:start u_Open\nfunc ExampleU() {\n\t// Output:\n}\n")
        _, _, probs = self.run_check("# empty\n", go=("u_test.go",))
        self.assertTrue(any("never closed" in p for p in probs), probs)

    def test_fix_rewrites_block(self):
        bad = HELLO_BLOCK.replace('"hello")', '"hullo")')
        text = "- step\n\n" + md_with("<!-- snippet: demo_test.go#demo_Hello -->", bad, indent="  ").split("\n", 2)[2]
        s, _, _ = self.run_check(text, fix=True)
        self.assertEqual(s, 1)
        fixed = snippets.read_text(os.path.join(self.root, "doc.md"))
        self.assertIn('  \tfmt.Println("hello")', fixed)
        self.assertEqual(self.problems_for(fixed), [])


class WrapTests(unittest.TestCase):
    def assertClean(self, text):
        self.assertEqual(wrapcheck.check_text(textwrap.dedent(text)), [])

    def assertWrapped(self, text, count=1):
        found = wrapcheck.check_text(textwrap.dedent(text))
        self.assertEqual(len(found), count, found)

    def test_wrapped_paragraph_fails(self):
        self.assertWrapped("One sentence that\ncontinues here.\n")

    def test_wrapped_list_item_fails(self):
        self.assertWrapped("- an item that\n  continues here\n- next item\n")

    def test_wrapped_blockquote_fails(self):
        self.assertWrapped("> quoted text that\n> continues here\n")

    def test_bold_label_then_text_fails(self):
        self.assertWrapped("**Label**\nText under it.\n")

    def test_structures_pass(self):
        self.assertClean("""\
            ---
            title: x
            about: y
            ---

            # Heading
            Paragraph right under a heading.

            - one
            - two
              - nested
            1. first
            2. second

            | a | b |
            |---|---|
            | 1 | 2 |

            ```go
            func a() {
            	b()
            }
            ```

            <!-- a comment
            over lines -->

            <p align="center">
              <a href="x">y</a>
            </p>

            > quote one
            >
            > quote two

            Setext heading
            ---

            [ref]: https://example.com
            [ref2]: https://example.com

            Line with hard break\\
            next line.
            """)

    def test_paragraph_then_list_passes(self):
        self.assertClean("Intro:\n- a\n- b\n")


class LinkTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = self.tmp.name
        with open(os.path.join(self.root, "a.md"), "w") as fh:
            fh.write("# Title\n\n## Some `Code` part\n")

    def tearDown(self):
        self.tmp.cleanup()

    def check(self, text):
        with open(os.path.join(self.root, "b.md"), "w") as fh:
            fh.write(text)
        return linkcheck.check_file("b.md", self.root)

    def test_good_links_pass(self):
        self.assertEqual(self.check("[a](a.md) [b](a.md#some-code-part) [c](https://x.invalid/nope) [d](#t)\n# T\n"), [])

    def test_missing_file_fails(self):
        self.assertEqual(len(self.check("[x](nope.md)\n")), 1)

    def test_bad_anchor_fails(self):
        self.assertEqual(len(self.check("[x](a.md#nope)\n")), 1)

    def test_docsync_citation_skipped(self):
        self.assertEqual(self.check("[`Save`](ds:block?id=save-abcdefgh)\n"), [])

    def test_ds_prefixed_path_still_checked(self):
        self.assertEqual(len(self.check("[x](ds/nope.md)\n")), 1)

    def test_link_in_code_fence_ignored(self):
        self.assertEqual(self.check("```md\n[x](nope.md)\n```\n"), [])


class WordTests(unittest.TestCase):
    def test_hits(self):
        text = "built by Claude\nsee /Users/x/y\nAI notes\nGenerated with a tool\nfine OPENAI_KEY and paint\n"
        hits = words.check_text(text, "f.md")
        self.assertEqual([h.split(":")[1] for h in hits], ["1", "2", "3", "4"])


if __name__ == "__main__":
    unittest.main()
