#include "msgfmt/msgfmt.hpp"

#include <iostream>
#include <iterator>
#include <string>
#include <string_view>

namespace {

void jsonString(std::ostream& out, std::string_view value) {
    constexpr char hex[] = "0123456789abcdef";
    out.put('"');
    for (unsigned char ch : value) {
        switch (ch) {
            case '"': out << "\\\""; break;
            case '\\': out << "\\\\"; break;
            case '\b': out << "\\b"; break;
            case '\f': out << "\\f"; break;
            case '\n': out << "\\n"; break;
            case '\r': out << "\\r"; break;
            case '\t': out << "\\t"; break;
            default:
                if (ch < 0x20) {
                    out << "\\u00" << hex[ch >> 4] << hex[ch & 0x0f];
                } else {
                    out.put(static_cast<char>(ch));
                }
        }
    }
    out.put('"');
}

void jsonResult(std::ostream& out, const msgfmt::Result& result) {
    out << "{\"text\":";
    jsonString(out, result.text);
    out << ",\"ranges\":[";
    bool first = true;
    for (const auto& range : result.ranges) {
        if (!first) {
            out.put(',');
        }
        first = false;
        out << "{\"style\":";
        jsonString(out, range.style);
        out << ",\"offset\":" << range.offset << ",\"length\":" << range.length << ",\"url\":";
        jsonString(out, range.url);
        out.put('}');
    }
    out << "]}\n";
}

}  // namespace

int main(int argc, char* argv[]) {
    if (argc > 2 || (argc == 2 && std::string_view(argv[1]) != "escape")) {
        std::cerr << "usage: msgfmt [escape]\n";
        return 2;
    }

    const std::string input{std::istreambuf_iterator<char>(std::cin), std::istreambuf_iterator<char>()};
    try {
        if (argc == 2) {
            std::cout << msgfmt::escape(input);
        } else {
            jsonResult(std::cout, msgfmt::parse(input));
        }
    } catch (const msgfmt::InvalidUtf8&) {
        std::cout << "{\"error\":\"invalid_utf8\"}\n";
        return 2;
    }
    return 0;
}
