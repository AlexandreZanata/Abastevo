#!/usr/bin/env python3
"""Validate the iOS String Catalogs and their Swift and Info.plist consumers."""

from collections import Counter
from collections.abc import Iterator
from dataclasses import dataclass
import json
from pathlib import Path
import plistlib
import re
import unittest

IOS_ROOT = Path(__file__).resolve().parents[2] / "iosApp"
LOCALES = {"en", "pt-BR", "es", "de", "fr", "ja", "ru", "zh-Hans"}
TARGET_CATALOGS = {
    "AnpFuelShell": "Localizable.xcstrings",
    "AnpFuelCore": "Resources/Localizable.xcstrings",
}
VIEW_TEXT_CALLS = {
    "Text",
    "Button",
    "Label",
    "ContentUnavailableView",
    "LabeledContent",
    "Link",
    "Picker",
    "ProgressView",
    "Section",
    "ShareLink",
    "Stepper",
    "TextField",
    "Toggle",
    "navigationTitle",
    "accessibilityLabel",
    "accessibilityHint",
    "accessibilityValue",
    "alert",
    "confirmationDialog",
}
# A file and literal may be added only when that text is intentionally identical
# in every locale, with the reason alongside the entry.
VERBATIM_TEXT = {
    ("DesignSystem.swift", "A"),  # Decorative brand monogram, hidden from VoiceOver.
}
FORMAT = re.compile(r"%(?:%|(?:(?P<position>[1-9]\d*)\$)?[-+#0']*\d*(?:\.\d+)?(?P<type>hh|ll|[hljztL])?(?P<conversion>[@diouxXfFeEgGaAcsp]))")
SWIFT_TOKEN = re.compile(r'//[^\n]*|/\*|(?P<quote>\#*(?:"""|"))|\w+|[^\s]')
COMMENT_MARKER = re.compile(r"/\*|\*/")


@dataclass(frozen=True)
class SwiftToken:
    text: str
    offset: int
    end: int
    is_string: bool = False
    interpolations: tuple[tuple[int, int], ...] = ()


def swift_tokens(source: str, start: int = 0, end: int | None = None) -> Iterator[SwiftToken]:
    """Read the strings and punctuation needed to identify localization call sites."""
    limit = len(source) if end is None else end
    while (match := SWIFT_TOKEN.search(source, start, limit)) is not None:
        start = match.end()
        token = match.group()
        if token.startswith("//"):
            continue
        if token == "/*":
            depth = 1
            for marker in COMMENT_MARKER.finditer(source, start, limit):
                depth += 1 if marker.group() == "/*" else -1
                if depth == 0:
                    start = marker.end()
                    break
            else:
                return
            continue
        if match.group("quote") is not None:
            string = swift_string(source, match, limit)
            yield string
            start = string.end
        else:
            yield SwiftToken(token, match.start(), start)


def swift_string(source: str, opening: re.Match[str], limit: int) -> SwiftToken:
    opener = opening.group()
    quote = opener.lstrip("#")
    hashes = opener[: len(opener) - len(quote)]
    closer = quote + hashes
    escape = "\\" + hashes
    text_parts = []
    interpolations = []
    start = cursor = opening.end()
    while cursor < limit:
        if source.startswith(closer, cursor):
            text_parts.append(source[start:cursor])
            return SwiftToken(
                "".join(text_parts), opening.start(), cursor + len(closer), True, tuple(interpolations)
            )
        if source.startswith(escape, cursor):
            escaped = cursor + len(escape)
            if source.startswith("(", escaped):
                text_parts.extend((source[start:cursor], " "))
                depth = 1
                for token in swift_tokens(source, escaped + 1, limit):
                    if token.is_string:
                        continue
                    if token.text == "(":
                        depth += 1
                    elif token.text == ")":
                        depth -= 1
                        if depth == 0:
                            interpolations.append((escaped + 1, token.offset))
                            start = cursor = token.end
                            break
                else:
                    break
            else:
                cursor = escaped + 1
            continue
        cursor += 1
    raise ValueError(f"unterminated Swift string on line {source.count(chr(10), 0, opening.start()) + 1}")


def localized_calls(source: str, start: int = 0, end: int | None = None):
    """Return literal localization keys and unlocalized strings in UI title arguments."""
    tokens = list(swift_tokens(source, start, end))
    for index, token in enumerate(tokens):
        if token.is_string:
            for lower, upper in token.interpolations:
                yield from localized_calls(source, lower, upper)
            continue
        if token.text == "String" and [
            next_token.text for next_token in tokens[index + 1 : index + 4]
        ] == ["(", "localized", ":"]:
            key = tokens[index + 4]
            if not key.is_string or key.interpolations:
                yield "dynamic_key", key
            else:
                yield "key", key
        if token.text not in VIEW_TEXT_CALLS or index + 1 >= len(tokens):
            continue
        if tokens[index + 1].text != "(":
            continue
        depth = 0
        for argument in tokens[index + 2 :]:
            if argument.is_string:
                if depth == 0 and any(char.isalpha() for char in argument.text):
                    yield "literal", argument
            elif argument.text in ("(", "[", "{"):
                depth += 1
            elif argument.text in (")", "]", "}"):
                if depth == 0:
                    break
                depth -= 1
            elif argument.text == "," and depth == 0:
                break


def placeholders(value: str) -> Counter[tuple[int, str]]:
    """Preserve argument indexes and types while allowing reordered translations."""
    formats = [match for match in FORMAT.finditer(value) if match.group("conversion") is not None]
    positioned = [match.group("position") is not None for match in formats]
    if any(positioned) and not all(positioned):
        raise ValueError("cannot mix positional and sequential format arguments")
    return Counter(
        (
            int(match.group("position") or index),
            (match.group("type") or "") + match.group("conversion"),
        )
        for index, match in enumerate(formats, start=1)
    )


def string_values(node: dict) -> Iterator[str]:
    unit = node.get("stringUnit")
    if unit is not None:
        yield unit["value"]
    for variants in node.get("variations", {}).values():
        for choice in variants.values():
            yield from string_values(choice)


class IosLocalizationTests(unittest.TestCase):
    def catalog(self, path: Path) -> dict:
        catalog = json.loads(path.read_text(encoding="utf-8"))
        self.assertEqual(catalog["sourceLanguage"], "en", path.name)
        self.assertTrue(catalog["strings"], f"{path}: empty catalog")
        return catalog["strings"]

    def check_translations(self, catalog: dict) -> None:
        for key, entry in catalog.items():
            with self.subTest(key=key):
                translations = entry.get("localizations", {})
                self.assertEqual(set(translations), LOCALES)
                english = list(string_values(translations["en"]))
                self.assertTrue(english)
                expected = placeholders(english[0])
                for locale, node in translations.items():
                    values = list(string_values(node))
                    self.assertTrue(values, f"{locale} has no string value")
                    for value in values:
                        self.assertTrue(value.strip(), f"{locale} has an empty string")
                        self.assertEqual(placeholders(value), expected, f"{locale} placeholders differ")

    def test_target_catalogs_cover_exactly_the_keys_used_by_swift(self):
        for target, relative_catalog in TARGET_CATALOGS.items():
            with self.subTest(target=target):
                folder = IOS_ROOT / "Sources" / target
                catalog = self.catalog(folder / relative_catalog)
                self.check_translations(catalog)
                used = set()
                problems = []
                for file in sorted(folder.rglob("*.swift")):
                    source = file.read_text(encoding="utf-8")
                    for kind, token in localized_calls(source):
                        if kind == "key":
                            used.add(token.text)
                        elif kind != "literal" or (file.name, token.text) not in VERBATIM_TEXT:
                            line = source.count("\n", 0, token.offset) + 1
                            problems.append(f"{file.name}:{line}: {kind}: {token.text!r}")
                missing = sorted(used - catalog.keys())
                unused = sorted(catalog.keys() - used)
                if missing:
                    problems.append(f"Missing catalog keys: {', '.join(missing)}")
                if unused:
                    problems.append(f"Unused catalog keys: {', '.join(unused)}")
                self.assertFalse(problems, "\n".join(problems))

    def test_info_plist_catalog_covers_display_name_and_usage_descriptions(self):
        app = IOS_ROOT / "App"
        with (app / "Info.plist").open("rb") as source:
            info = plistlib.load(source)
        self.assertEqual(info["CFBundleDevelopmentRegion"], "en")
        self.assertEqual(set(info["CFBundleLocalizations"]), LOCALES)
        self.assertEqual(len(info["CFBundleLocalizations"]), len(LOCALES))
        catalog = self.catalog(app / "InfoPlist.xcstrings")
        descriptions = {key for key in info if key.startswith("NS") and key.endswith("UsageDescription")}
        self.assertEqual(set(catalog), descriptions | {"CFBundleDisplayName"})
        self.check_translations(catalog)
        for key in catalog:
            self.assertEqual(list(string_values(catalog[key]["localizations"]["en"])), [info[key]], key)

    def test_xcode_catalogs_are_resources_of_their_own_targets(self):
        project = (IOS_ROOT / "AnpFuel.xcodeproj/project.pbxproj").read_text(encoding="utf-8")
        file_paths = dict(re.findall(
            r'(?m)^\t\t([A-F0-9]{24}) /\* [^\n]+? \*/ = \{isa = PBXFileReference; '
            r'lastKnownFileType = text\.json\.xcstrings; [^\n]+?path = "([^"]+)";',
            project,
        ))
        build_refs = dict(re.findall(
            r"(?m)^\t\t([A-F0-9]{24}) /\* [^\n]+?\.xcstrings in Resources \*/ = "
            r"\{isa = PBXBuildFile; fileRef = ([A-F0-9]{24})",
            project,
        ))
        phases = dict(re.findall(
            r"(?ms)^\t\t([A-F0-9]{24}) /\* Resources \*/ = "
            r"\{isa = PBXResourcesBuildPhase; [^\n]+?files = \(\n(.*?)^\t\t\);",
            project,
        ))
        targets = dict(re.findall(
            r"(?ms)^\t\t[A-F0-9]{24} /\* ([^\n]+?) \*/ = \{\n\t\t\tisa = PBXNativeTarget;\n"
            r".*?buildPhases = \(\n(.*?)^\t\t\t\);",
            project,
        ))
        expected = {
            "AnpFuel": ["InfoPlist.xcstrings", "Localizable.xcstrings"],
            "AnpFuelCore": ["Resources/Localizable.xcstrings"],
            "AnpFuelCoreTests": [],
        }
        self.assertEqual(set(targets), set(expected))
        for target, expected_paths in expected.items():
            with self.subTest(target=target):
                phase_ids = re.findall(r"[A-F0-9]{24}", targets[target])
                builds = re.findall(r"[A-F0-9]{24}", "".join(phases.get(phase, "") for phase in phase_ids))
                actual = [file_paths[build_refs[build]] for build in builds if build in build_refs]
                self.assertCountEqual(actual, expected_paths)

    def test_placeholders_preserve_types_and_allow_positional_reordering(self):
        self.assertEqual(placeholders("%@ · %lld · 5%%"), placeholders("%2$lld · %1$@ · 5%%"))
        self.assertNotEqual(placeholders("%@ · %lld"), placeholders("%1$lld · %2$@"))
        self.assertNotEqual(placeholders("%@ · %lld"), placeholders("%@"))
        with self.assertRaises(ValueError):
            placeholders("%1$@ · %lld")

    def test_source_scan_ignores_comments_and_detects_nested_calls_and_literals(self):
        source = r'''
        // Text("Ignored line comment")
        /* Text("Ignored /* nested */ block comment") */
        Text(String(localized: "navigation.stop", defaultValue: "Stop"))
        Label("\(String(localized: "navigation.map"))", systemImage: "map")
        Text(verbatim: "R$ \(amount)")
        Button(condition ? "Stop" : #"""Continue"""#) {}
        .navigationTitle(String(localized: dynamicKey))
        Text("\(amount)")
        '''
        scanned = [(kind, token.text) for kind, token in localized_calls(source)]
        self.assertEqual(
            scanned,
            [("key", "navigation.stop"), ("key", "navigation.map"), ("literal", "R$  "),
             ("literal", "Stop"), ("literal", "Continue"), ("dynamic_key", "dynamicKey")],
        )


if __name__ == "__main__":
    unittest.main(verbosity=2)
