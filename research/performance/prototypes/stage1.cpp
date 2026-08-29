// Ceiling benchmark for a SIMD JSON structural pass over evidence payloads.
//
// The Go analyzer's cost after pass fusion is dominated by encoding/json:
// checkValid is 26% of analyzer CPU and is provably redundant on payloads whose
// enclosing line was already validated, and decodeState.skip is another 14%
// walking fields nobody asked for. Both are byte-scanning work, which is what
// SIMD is for.
//
// This measures the two things a production cgo submodule would actually do:
//
//   stage1        - simdjson-style structural/quote bitmap over the buffer.
//                   This is the floor cost of any SIMD JSON work.
//   extract       - locate a fixed set of top-level keys and return the byte
//                   range of each value, which is what a Go caller needs in
//                   order to parse only the scalars it cares about.
//
// It is a ceiling measurement, not a proposed parser: it does not implement
// full JSON validation, and the semantic-equivalence bar for replacing
// encoding/json is separate and much higher.
//
// Build: g++ -std=c++23 -O3 -march=native -o stage1 stage1.cpp

#include <immintrin.h>
#include <wmmintrin.h>

#include <algorithm>
#include <chrono>
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <fstream>
#include <string>
#include <string_view>
#include <vector>

using Clock = std::chrono::steady_clock;

// prefix_xor turns a bitmask of quote positions into a mask of "inside string"
// positions: bit i is set when an odd number of quotes occurred at or before i.
// Carry-less multiplication by all-ones computes that prefix XOR in one
// instruction, which is the trick simdjson uses.
static inline uint64_t prefix_xor(uint64_t bits) {
  __m128i all_ones = _mm_set1_epi8(static_cast<char>(0xFF));
  __m128i result = _mm_clmulepi64_si128(_mm_set_epi64x(0ULL, static_cast<long long>(bits)), all_ones, 0);
  return static_cast<uint64_t>(_mm_cvtsi128_si64(result));
}

static inline uint64_t block_eq_mask(const uint8_t* p, uint8_t c) {
  const __m256i needle = _mm256_set1_epi8(static_cast<char>(c));
  const __m256i lo = _mm256_loadu_si256(reinterpret_cast<const __m256i*>(p));
  const __m256i hi = _mm256_loadu_si256(reinterpret_cast<const __m256i*>(p + 32));
  const uint64_t a = static_cast<uint32_t>(_mm256_movemask_epi8(_mm256_cmpeq_epi8(lo, needle)));
  const uint64_t b = static_cast<uint32_t>(_mm256_movemask_epi8(_mm256_cmpeq_epi8(hi, needle)));
  return a | (b << 32);
}

// classify_structural marks {,},[,],:,, positions in one 64-byte block.
static inline uint64_t block_structural_mask(const uint8_t* p) {
  static const __m256i low_table = _mm256_setr_epi8(
      // low nibble -> bitset of structural high nibbles, simdjson-style table
      16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 1, 2, 1, 0, 0,
      16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 1, 2, 1, 0, 0);
  static const __m256i high_table = _mm256_setr_epi8(
      8, 0, 18, 4, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0,
      8, 0, 18, 4, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0);
  auto lane = [&](const __m256i chunk) -> uint32_t {
    const __m256i low = _mm256_and_si256(chunk, _mm256_set1_epi8(0x0F));
    const __m256i high = _mm256_and_si256(_mm256_srli_epi32(chunk, 4), _mm256_set1_epi8(0x0F));
    const __m256i v = _mm256_and_si256(_mm256_shuffle_epi8(low_table, low),
                                       _mm256_shuffle_epi8(high_table, high));
    return static_cast<uint32_t>(
        _mm256_movemask_epi8(_mm256_cmpgt_epi8(v, _mm256_setzero_si256())));
  };
  const __m256i lo = _mm256_loadu_si256(reinterpret_cast<const __m256i*>(p));
  const __m256i hi = _mm256_loadu_si256(reinterpret_cast<const __m256i*>(p + 32));
  return static_cast<uint64_t>(lane(lo)) | (static_cast<uint64_t>(lane(hi)) << 32);
}

// Stage1 holds the structural positions of one buffer.
struct Stage1 {
  std::vector<uint32_t> structurals;
  uint64_t quote_positions = 0;
};

// run_stage1 computes the structural index of buf, excluding characters inside
// strings. Escapes are handled by the odd-backslash-run rule.
static void run_stage1(const uint8_t* buf, size_t len, std::vector<uint32_t>& out) {
  out.clear();
  uint64_t prev_inside_string = 0;
  uint64_t prev_escape_carry = 0;
  size_t i = 0;
  for (; i + 64 <= len; i += 64) {
    const uint64_t backslashes = block_eq_mask(buf + i, '\\');
    const uint64_t quotes_raw = block_eq_mask(buf + i, '"');

    // Positions escaped by a preceding odd run of backslashes.
    uint64_t starts = backslashes & ~((backslashes << 1) | prev_escape_carry);
    uint64_t even_start_mask = 0x5555555555555555ULL;
    uint64_t odd_starts = starts & ~even_start_mask;
    uint64_t even_starts = starts & even_start_mask;
    uint64_t escaped = ((backslashes + even_starts) & ~backslashes) & even_start_mask;
    escaped |= ((backslashes + odd_starts) & ~backslashes) & ~even_start_mask;
    prev_escape_carry = (backslashes >> 63) & 1ULL;

    const uint64_t quotes = quotes_raw & ~escaped;
    uint64_t inside = prefix_xor(quotes) ^ prev_inside_string;
    prev_inside_string = static_cast<uint64_t>(0) - (inside >> 63);

    uint64_t structural = block_structural_mask(buf + i) & ~inside;
    while (structural) {
      out.push_back(static_cast<uint32_t>(i + static_cast<size_t>(__builtin_ctzll(structural))));
      structural &= structural - 1;
    }
  }
  // Scalar tail.
  bool inside_string = prev_inside_string != 0;
  bool escape = prev_escape_carry != 0;
  for (; i < len; ++i) {
    const uint8_t c = buf[i];
    if (escape) { escape = false; continue; }
    if (c == '\\') { escape = true; continue; }
    if (c == '"') { inside_string = !inside_string; continue; }
    if (inside_string) continue;
    if (c == '{' || c == '}' || c == '[' || c == ']' || c == ':' || c == ',') {
      out.push_back(static_cast<uint32_t>(i));
    }
  }
}

// extract_top_level_fields walks one JSON object's structural positions and
// records the value byte range for each requested key. This is the shape a Go
// caller would use: it parses only the small scalars it actually reads.
struct FieldRange { uint32_t start = 0; uint32_t end = 0; bool found = false; };

static void extract_top_level_fields(const uint8_t* buf, size_t len,
                                     const std::vector<uint32_t>& structurals,
                                     const std::vector<std::string>& keys,
                                     std::vector<FieldRange>& out) {
  out.assign(keys.size(), FieldRange{});
  if (structurals.empty() || buf[structurals[0]] != '{') return;
  int depth = 0;
  for (size_t s = 0; s < structurals.size(); ++s) {
    const uint32_t at = structurals[s];
    const uint8_t c = buf[at];
    if (c == '{' || c == '[') { ++depth; continue; }
    if (c == '}' || c == ']') { --depth; continue; }
    if (c != ':' || depth != 1) continue;
    // The key is the string ending just before this colon.
    uint32_t keyEnd = at;
    while (keyEnd > 0 && buf[keyEnd - 1] != '"') --keyEnd;
    uint32_t keyStart = keyEnd - 1;
    while (keyStart > 0 && buf[keyStart - 1] != '"') --keyStart;
    std::string_view key(reinterpret_cast<const char*>(buf + keyStart), keyEnd - 1 - keyStart);
    // The value runs to the next depth-1 comma or the closing brace.
    uint32_t valueStart = at + 1;
    while (valueStart < len && (buf[valueStart] == ' ' || buf[valueStart] == '\t')) ++valueStart;
    uint32_t valueEnd = static_cast<uint32_t>(len);
    int localDepth = 0;
    for (size_t t = s + 1; t < structurals.size(); ++t) {
      const uint8_t d = buf[structurals[t]];
      if (d == '{' || d == '[') { ++localDepth; continue; }
      if (d == '}' || d == ']') {
        if (localDepth == 0) { valueEnd = structurals[t]; break; }
        --localDepth;
        continue;
      }
      if (d == ',' && localDepth == 0) { valueEnd = structurals[t]; break; }
    }
    for (size_t k = 0; k < keys.size(); ++k) {
      if (out[k].found) continue;
      if (key == keys[k]) { out[k] = FieldRange{valueStart, valueEnd, true}; break; }
    }
  }
}

int main(int argc, char** argv) {
  if (argc < 2) { fprintf(stderr, "usage: stage1 <corpus.jsonl>\n"); return 2; }
  std::ifstream input(argv[1], std::ios::binary);
  std::string blob((std::istreambuf_iterator<char>(input)), std::istreambuf_iterator<char>());
  // Pad so 64-byte loads never read past the end.
  blob.append(128, ' ');
  const uint8_t* base = reinterpret_cast<const uint8_t*>(blob.data());

  // Extract the innermost payloads the way the Go analyzer does, so the
  // comparison is against the same bytes.
  std::vector<std::pair<uint32_t, uint32_t>> payloads;
  size_t start = 0, usable = blob.size() - 128;
  size_t payloadBytes = 0;
  for (size_t i = 0; i < usable && payloads.size() < 300000; ++i) {
    if (blob[i] != '\n') continue;
    std::string_view line(blob.data() + start, i - start);
    size_t at = line.find("\"payload\":");
    start = i + 1;
    if (at == std::string_view::npos) continue;
    size_t open = line.find('{', at);
    if (open == std::string_view::npos) continue;
    int depth = 0; size_t close = open;
    bool inStr = false, esc = false;
    for (size_t j = open; j < line.size(); ++j) {
      char c = line[j];
      if (esc) { esc = false; continue; }
      if (c == '\\') { esc = true; continue; }
      if (c == '"') { inStr = !inStr; continue; }
      if (inStr) continue;
      if (c == '{') ++depth;
      else if (c == '}') { if (--depth == 0) { close = j; break; } }
    }
    if (depth != 0) continue;
    uint32_t s = static_cast<uint32_t>((line.data() - blob.data()) + open);
    uint32_t e = static_cast<uint32_t>((line.data() - blob.data()) + close + 1);
    payloads.emplace_back(s, e);
    payloadBytes += e - s;
  }
  printf("payloads: %zu, %.2f MiB\n", payloads.size(), static_cast<double>(payloadBytes) / (1024 * 1024));

  std::vector<std::string> keys;
  for (int a = 2; a < argc; ++a) keys.emplace_back(argv[a]);
  if (keys.empty()) keys = {"symbol", "payload", "side", "price", "timestamp"};
  printf("keys:"); for (auto& k : keys) printf(" %s", k.c_str()); printf("\n");
  std::vector<uint32_t> structurals;
  std::vector<FieldRange> fields;

  const int reps = 5;
  double bestStage1 = 1e18, bestExtract = 1e18;
  size_t checksum = 0;
  for (int r = 0; r < reps; ++r) {
    auto t0 = Clock::now();
    size_t total = 0;
    for (auto [s, e] : payloads) {
      run_stage1(base + s, e - s, structurals);
      total += structurals.size();
    }
    auto t1 = Clock::now();
    bestStage1 = std::min(bestStage1, std::chrono::duration<double>(t1 - t0).count());
    checksum = total;
  }
  size_t found = 0;
  std::vector<size_t> perKey(keys.size(), 0);
  for (int r = 0; r < reps; ++r) {
    std::fill(perKey.begin(), perKey.end(), 0);
    auto t0 = Clock::now();
    size_t hits = 0;
    for (auto [s, e] : payloads) {
      run_stage1(base + s, e - s, structurals);
      extract_top_level_fields(base + s, e - s, structurals, keys, fields);
      for (size_t k = 0; k < fields.size(); ++k) if (fields[k].found) { ++hits; ++perKey[k]; }
    }
    auto t1 = Clock::now();
    bestExtract = std::min(bestExtract, std::chrono::duration<double>(t1 - t0).count());
    found = hits;
  }

  const double mib = static_cast<double>(payloadBytes) / (1024 * 1024);
  printf("stage1_only     %8.4f s  %9.1f MiB/s  (structurals=%zu)\n", bestStage1, mib / bestStage1, checksum);
  printf("stage1+extract  %8.4f s  %9.1f MiB/s  (fields found=%zu)\n", bestExtract, mib / bestExtract, found);
  for (size_t k = 0; k < keys.size(); ++k) printf("  key %-14s present in %zu payloads\n", keys[k].c_str(), perKey[k]);
  return 0;
}
