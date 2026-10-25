#pragma once

#include <cstddef>
#include <stdexcept>
#include <string>
#include <string_view>
#include <vector>

namespace msgfmt {

struct Range {
    std::string style;
    std::size_t offset;
    std::size_t length;
    std::string url;
};

struct Result {
    std::string text;
    std::vector<Range> ranges;
};

class InvalidUtf8 final : public std::runtime_error {
public:
    InvalidUtf8() : std::runtime_error("invalid_utf8") {}
};

// Both functions validate the complete input before producing a result.
// Invalid UTF-8 is reported with InvalidUtf8.
Result parse(std::string_view input);
std::string escape(std::string_view input);

}  // namespace msgfmt
