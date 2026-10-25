#include "msgfmt/msgfmt.hpp"

#include <cstdlib>
#include <iostream>
#include <string>
#include <string_view>
#include <vector>

namespace {

int failures = 0;

void expect(std::string_view name, std::string_view input, std::string_view text,
            std::vector<msgfmt::Range> ranges) {
    const auto actual = msgfmt::parse(input);
    if (actual.text != text || actual.ranges.size() != ranges.size()) {
        std::cerr << name << ": text/range count mismatch\n";
        ++failures;
        return;
    }
    for (std::size_t index = 0; index < ranges.size(); ++index) {
        const auto& got = actual.ranges[index];
        const auto& want = ranges[index];
        if (got.style != want.style || got.offset != want.offset || got.length != want.length || got.url != want.url) {
            std::cerr << name << ": range " << index << " mismatch: " << got.style << '(' << got.offset
                      << ", " << got.length << ", " << got.url << ")\n";
            ++failures;
        }
    }
}

void expectInvalid(std::string_view name, std::string_view input) {
    bool parseRejected = false;
    bool escapeRejected = false;
    try {
        (void)msgfmt::parse(input);
    } catch (const msgfmt::InvalidUtf8&) {
        parseRejected = true;
    }
    try {
        (void)msgfmt::escape(input);
    } catch (const msgfmt::InvalidUtf8&) {
        escapeRejected = true;
    }
    if (!parseRejected || !escapeRejected) {
        std::cerr << name << ": invalid UTF-8 accepted\n";
        ++failures;
    }
}

}  // namespace

int main() {
    // The fifteen control examples from Appendix A of TASK.md.
    expect("1 bold", "**важно**", "важно", {{"bold", 0, 5, ""}});
    expect("2 prefix", "Привет, **мир**!", "Привет, мир!", {{"bold", 8, 3, ""}});
    expect("3 italic", "_курсив_", "курсив", {{"italic", 0, 6, ""}});
    expect("4 word boundary", "a_b_c", "a_b_c", {});
    expect("5 unmatched", "**не закрыто", "**не закрыто", {});
    expect("6 escaped star", "2 \\* 3", "2 * 3", {});
    expect("7 code", "`чат * список`", "чат * список", {{"code", 0, 12, ""}});
    expect("8 link", "[документация](https://docs.github.com)", "документация",
           {{"link", 0, 12, "https://docs.github.com"}});
    expect("9 nested", "**жирный _и_ курсив**", "жирный и курсив",
           {{"bold", 0, 15, ""}, {"italic", 7, 1, ""}});
    expect("10 emoji prefix", "🚀 **ок**", "🚀 ок", {{"bold", 3, 2, ""}});
    expect("11 emoji inside", "**🚀 ок**", "🚀 ок", {{"bold", 0, 5, ""}});
    expect("12 empty URL", "[текст]()", "[текст]()", {});
    expect("13 escaped slash", "\\\\", "\\", {});
    expect("14 empty", "", "", {});
    expect("15 opener followed by space", "_ e_", "_ e_", {});

    expect("unmatched nested", "**open _x_", "**open _x_", {});
    expect("same style literal", "**a ** b**", "a ** b", {{"bold", 0, 6, ""}});
    expect("nearest close", "**a**b**", "ab**", {{"bold", 0, 1, ""}});
    expect("nested code is raw", "**a `*\\b` c**", "a *\\b c",
           {{"bold", 0, 7, ""}, {"code", 2, 3, ""}});
    expect("link with bold", "[**x**](u)", "x", {{"link", 0, 1, "u"}, {"bold", 0, 1, ""}});
    expect("bold with link", "**[x](u)**", "x", {{"bold", 0, 1, ""}, {"link", 0, 1, "u"}});
    expect("bracket in link text", "[a\\]b](u)", "[a\\]b](u)", {});
    expect("bad URL whitespace", "[a](bad url)", "[a](bad url)", {});
    expect("missing URL close", "[a](bad", "[a](bad", {});
    expect("line break is text", "**a\nb**", "a\nb", {{"bold", 0, 3, ""}});
    expect("underscore after emoji", "🚀_x_", "🚀x", {{"italic", 2, 1, ""}});
    expect("decomposed codepoint", "**й**", "й", {{"bold", 0, 2, ""}});
    expect("control character", "**a\x01" "b**", "a\x01" "b", {{"bold", 0, 3, ""}});
    expect("control character after opener", "**\x1c" "x**", "\x1c" "x", {{"bold", 0, 2, ""}});
    expect("raw backslash in code", "`\\*`", "\\*", {{"code", 0, 2, ""}});
    expect("incomplete nesting does not cross", "**a _b** c_", "a _b c_", {{"bold", 0, 4, ""}});

    const std::string specials = "*_`[]()\\";
    const std::string escaped = "\\*\\_\\`\\[\\]\\(\\)\\\\";
    if (msgfmt::escape(specials) != escaped || msgfmt::escape("2 * 3") != "2 \\* 3" ||
        msgfmt::escape("й🚀") != "й🚀" || msgfmt::escape("") != "") {
        std::cerr << "escape mismatch\n";
        ++failures;
    }
    for (const std::string& original : {std::string("a_b_c"), std::string("[x](u) **bold**"),
                                       std::string("\\`🚀_[]()"), std::string("line\n\x01"),
                                       std::string("\0*", 2)}) {
        const auto roundTrip = msgfmt::parse(msgfmt::escape(original));
        if (roundTrip.text != original || !roundTrip.ranges.empty()) {
            std::cerr << "escape round trip mismatch\n";
            ++failures;
        }
    }

    expectInvalid("truncated", std::string({static_cast<char>(0xe2), static_cast<char>(0x82)}));
    expectInvalid("overlong", std::string({static_cast<char>(0xc0), static_cast<char>(0xaf)}));
    expectInvalid("surrogate", std::string({static_cast<char>(0xed), static_cast<char>(0xa0), static_cast<char>(0x80)}));
    expectInvalid("too large", std::string({static_cast<char>(0xf4), static_cast<char>(0x90), static_cast<char>(0x80), static_cast<char>(0x80)}));
    expectInvalid("lone continuation", std::string(1, static_cast<char>(0x80)));

    if (failures != 0) {
        std::cerr << failures << " test(s) failed\n";
        return EXIT_FAILURE;
    }
    return EXIT_SUCCESS;
}
