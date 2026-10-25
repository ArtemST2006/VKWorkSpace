#include "msgfmt/msgfmt.hpp"

#include "unicode_data.hpp"

#include <algorithm>
#include <cstdint>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

namespace msgfmt {
namespace {

struct Rune {
    std::uint32_t codepoint;
    std::size_t begin;
    std::size_t end;
};

std::vector<Rune> decode(std::string_view input) {
    std::vector<Rune> result;
    for (std::size_t pos = 0; pos < input.size();) {
        const auto first = static_cast<unsigned char>(input[pos]);
        std::size_t width = 0;
        std::uint32_t value = 0;
        std::uint32_t minimum = 0;
        if (first < 0x80) {
            width = 1;
            value = first;
        } else if (first >= 0xc2 && first <= 0xdf) {
            width = 2;
            value = first & 0x1f;
            minimum = 0x80;
        } else if (first >= 0xe0 && first <= 0xef) {
            width = 3;
            value = first & 0x0f;
            minimum = 0x800;
        } else if (first >= 0xf0 && first <= 0xf4) {
            width = 4;
            value = first & 0x07;
            minimum = 0x10000;
        } else {
            throw InvalidUtf8();
        }
        if (width > input.size() - pos) {
            throw InvalidUtf8();
        }
        for (std::size_t index = 1; index < width; ++index) {
            const auto next = static_cast<unsigned char>(input[pos + index]);
            if ((next & 0xc0) != 0x80) {
                throw InvalidUtf8();
            }
            value = (value << 6) | (next & 0x3f);
        }
        if (value < minimum || (value >= 0xd800 && value <= 0xdfff) || value > 0x10ffff) {
            throw InvalidUtf8();
        }
        result.push_back({value, pos, pos + width});
        pos += width;
    }
    return result;
}

bool space(std::uint32_t cp) {
    return detail::in_intervals(cp, detail::space_intervals);
}

bool word(std::uint32_t cp) {
    return detail::in_intervals(cp, detail::word_intervals);
}

bool service(std::uint32_t cp) {
    switch (cp) {
        case '*': case '_': case '`': case '[': case ']': case '(': case ')': case '\\':
            return true;
        default:
            return false;
    }
}

std::size_t utf16Width(std::uint32_t cp) {
    return cp > 0xffff ? 2 : 1;
}

enum class Style { bold, italic, code, link };

const char* styleName(Style style) {
    switch (style) {
        case Style::bold: return "bold";
        case Style::italic: return "italic";
        case Style::code: return "code";
        case Style::link: return "link";
    }
    return "";
}

struct Token {
    std::string bytes;
    std::size_t units;
    bool visible;
};

struct Frame {
    Style style;
    std::size_t openToken;
    std::size_t contentStart;
    std::size_t openRuneEnd;
    std::size_t order;
    bool hasInnerRightBracket;
};

struct Span {
    Style style;
    std::size_t openToken;
    std::size_t contentStart;
    std::size_t contentEnd;
    std::size_t order;
    std::string url;
};

class Parser {
public:
    Parser(std::string_view input, std::vector<Rune> runes)
        : input_(input), runes_(std::move(runes)) {}

    Result run() {
        std::size_t pos = 0;
        while (pos < runes_.size()) {
            const auto cp = runes_[pos].codepoint;

            // Code is raw: even a backslash before a backtick is ordinary text.
            if (!frames_.empty() && frames_.back().style == Style::code) {
                if (cp == ']' && active(Style::link) != frames_.size()) {
                    frames_[active(Style::link)].hasInnerRightBracket = true;
                }
                if (cp == '`' && beforeNonspace(pos)) {
                    close(Style::code, pos, pos + 1, "");
                } else {
                    emit(pos, pos + 1, true);
                }
                ++pos;
                continue;
            }

            if (cp == '\\' && pos + 1 < runes_.size() && service(runes_[pos + 1].codepoint)) {
                if (runes_[pos + 1].codepoint == ']' && active(Style::link) != frames_.size()) {
                    frames_[active(Style::link)].hasInnerRightBracket = true;
                }
                emit(pos, pos + 1, false);
                emit(pos + 1, pos + 2, true);
                pos += 2;
                continue;
            }

            if (cp == ']' && active(Style::link) != frames_.size()) {
                pos = finishLink(pos);
                continue;
            }

            if (cp == '*' && pos + 1 < runes_.size() && runes_[pos + 1].codepoint == '*') {
                if (active(Style::bold) != frames_.size()) {
                    if (beforeNonspace(pos)) {
                        close(Style::bold, pos, pos + 2, "");
                    } else {
                        emit(pos, pos + 2, true);
                    }
                } else if (afterNonspace(pos + 2)) {
                    open(Style::bold, pos, pos + 2);
                } else {
                    emit(pos, pos + 2, true);
                }
                pos += 2;
                continue;
            }

            if (cp == '_') {
                if (active(Style::italic) != frames_.size()) {
                    if (beforeNonspace(pos) && (pos + 1 == runes_.size() || !word(runes_[pos + 1].codepoint))) {
                        close(Style::italic, pos, pos + 1, "");
                    } else {
                        emit(pos, pos + 1, true);
                    }
                } else if ((pos == 0 || !word(runes_[pos - 1].codepoint)) && afterNonspace(pos + 1)) {
                    open(Style::italic, pos, pos + 1);
                } else {
                    emit(pos, pos + 1, true);
                }
                ++pos;
                continue;
            }

            if (cp == '`') {
                if (afterNonspace(pos + 1)) {
                    open(Style::code, pos, pos + 1);
                } else {
                    emit(pos, pos + 1, true);
                }
                ++pos;
                continue;
            }

            if (cp == '[') {
                if (active(Style::link) == frames_.size() && afterNonspace(pos + 1)) {
                    open(Style::link, pos, pos + 1);
                } else {
                    emit(pos, pos + 1, true);
                }
                ++pos;
                continue;
            }

            emit(pos, pos + 1, true);
            ++pos;
        }

        if (!frames_.empty()) {
            invalidateFrom(frames_.front().openToken);
        }
        return result();
    }

private:
    bool beforeNonspace(std::size_t pos) const {
        return pos > 0 && !space(runes_[pos - 1].codepoint);
    }

    bool afterNonspace(std::size_t pos) const {
        return pos < runes_.size() && !space(runes_[pos].codepoint);
    }

    std::size_t active(Style style) const {
        for (std::size_t index = frames_.size(); index > 0; --index) {
            if (frames_[index - 1].style == style) {
                return index - 1;
            }
        }
        return frames_.size();
    }

    void emit(std::size_t begin, std::size_t end, bool visible) {
        if (begin == end) {
            return;
        }
        std::size_t units = 0;
        for (std::size_t index = begin; index < end; ++index) {
            units += utf16Width(runes_[index].codepoint);
        }
        const auto byteBegin = runes_[begin].begin;
        const auto byteEnd = runes_[end - 1].end;
        tokens_.push_back({std::string(input_.substr(byteBegin, byteEnd - byteBegin)), units, visible});
    }

    void open(Style style, std::size_t begin, std::size_t end) {
        const auto token = tokens_.size();
        emit(begin, end, false);
        frames_.push_back({style, token, tokens_.size(), end, nextOrder_++, false});
    }

    // Unmatched constructs are restored as literal input, including escapes
    // and completed styles nested inside them.
    void invalidateFrom(std::size_t token) {
        for (std::size_t index = token; index < tokens_.size(); ++index) {
            tokens_[index].visible = true;
        }
        spans_.erase(std::remove_if(spans_.begin(), spans_.end(), [token](const Span& span) {
            return span.openToken >= token;
        }), spans_.end());
    }

    void close(Style style, std::size_t begin, std::size_t end, std::string url) {
        const auto index = active(style);
        const Frame frame = frames_[index];
        if (begin == frame.openRuneEnd) {
            invalidateFrom(frame.openToken);
            frames_.resize(index);
            emit(begin, end, true);
            return;
        }
        if (index + 1 < frames_.size()) {
            invalidateFrom(frames_[index + 1].openToken);
            frames_.resize(index + 1);
        }
        const auto contentEnd = tokens_.size();
        emit(begin, end, false);
        spans_.push_back({style, frame.openToken, frame.contentStart, contentEnd, frame.order, std::move(url)});
        frames_.pop_back();
    }

    std::size_t finishLink(std::size_t pos) {
        const auto index = active(Style::link);
        const Frame frame = frames_[index];
        std::size_t end = pos + 1;
        bool valid = false;
        std::string url;
        if (end < runes_.size() && runes_[end].codepoint == '(') {
            std::size_t cursor = end + 1;
            bool urlValid = cursor < runes_.size();
            while (cursor < runes_.size() && runes_[cursor].codepoint != ')') {
                if (space(runes_[cursor].codepoint)) {
                    urlValid = false;
                }
                ++cursor;
            }
            if (cursor < runes_.size()) {
                end = cursor + 1;
                if (urlValid && cursor > pos + 2) {
                    const auto byteBegin = runes_[pos + 2].begin;
                    url = std::string(input_.substr(byteBegin, runes_[cursor].begin - byteBegin));
                    valid = true;
                }
            } else {
                end = cursor;
            }
        }
        if (valid && beforeNonspace(pos) && pos > frame.openRuneEnd && !frame.hasInnerRightBracket) {
            close(Style::link, pos, end, std::move(url));
        } else {
            invalidateFrom(frame.openToken);
            frames_.resize(index);
            emit(pos, end, true);
        }
        return end;
    }

    Result result() const {
        Result output;
        std::vector<std::size_t> offsets(tokens_.size() + 1, 0);
        for (std::size_t index = 0; index < tokens_.size(); ++index) {
            offsets[index + 1] = offsets[index] + (tokens_[index].visible ? tokens_[index].units : 0);
            if (tokens_[index].visible) {
                output.text += tokens_[index].bytes;
            }
        }
        struct OrderedRange {
            Range range;
            std::size_t order;
        };
        std::vector<OrderedRange> ordered;
        ordered.reserve(spans_.size());
        for (const auto& span : spans_) {
            ordered.push_back({{styleName(span.style), offsets[span.contentStart],
                                offsets[span.contentEnd] - offsets[span.contentStart], span.url}, span.order});
        }
        std::sort(ordered.begin(), ordered.end(), [](const OrderedRange& left, const OrderedRange& right) {
            if (left.range.offset != right.range.offset) {
                return left.range.offset < right.range.offset;
            }
            if (left.range.length != right.range.length) {
                return left.range.length > right.range.length;
            }
            return left.order < right.order;
        });
        for (auto& item : ordered) {
            output.ranges.push_back(std::move(item.range));
        }
        return output;
    }

    std::string_view input_;
    std::vector<Rune> runes_;
    std::vector<Token> tokens_;
    std::vector<Frame> frames_;
    std::vector<Span> spans_;
    std::size_t nextOrder_ = 0;
};

}  // namespace

Result parse(std::string_view input) {
    return Parser(input, decode(input)).run();
}

std::string escape(std::string_view input) {
    const auto runes = decode(input);
    std::string output;
    for (const auto& rune : runes) {
        if (service(rune.codepoint)) {
            output.push_back('\\');
        }
        output.append(input.substr(rune.begin, rune.end - rune.begin));
    }
    return output;
}

}  // namespace msgfmt
