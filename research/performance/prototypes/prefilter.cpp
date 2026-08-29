// Ceiling benchmark for the analyzer's multi-needle line prefilter.
//
// The Go analyzer decides, per JSONL line, whether any of N quoted event names
// occurs anywhere in the line. This measures how fast that decision can be made
// at all on this host, so the Go implementation can be judged against a real
// ceiling rather than against intuition.
//
// Variants:
//   scalar_naive  - byte-at-a-time, mirroring analysis.bytesContains
//   scalar_memmem - one std::search / memmem per needle
//   simd_teddy    - AVX2 first/second byte candidate filter, verified scalar
//
// Build: g++ -std=c++23 -O3 -march=native -o prefilter prefilter.cpp

#include <immintrin.h>

#include <algorithm>
#include <chrono>
#include <cstring>
#include <cstdio>
#include <fstream>
#include <string>
#include <string_view>
#include <vector>

using Clock = std::chrono::steady_clock;

static std::vector<std::string> kNeedles = {
    "\"OrderAccepted\"", "\"OrderFill\"",   "\"OrderCancelled\"",
    "\"OrderRejected\"", "\"Trade\"",       "\"BookDelta\"",
    "\"balance_change\"", "\"position_update\"", "\"realized_pnl\"",
    "\"mark_price_update\"", "\"funding_rate_update\"", "\"open_interest\"",
};

// scalar_naive mirrors the Go analyzer's hand-rolled search exactly.
static bool contains_naive(const char* hay, size_t hn, const char* nee, size_t nn) {
  if (nn == 0 || nn > hn) return false;
  const char first = nee[0];
  const size_t limit = hn - nn;
  for (size_t i = 0; i <= limit; ++i) {
    if (hay[i] != first) continue;
    bool match = true;
    for (size_t j = 1; j < nn; ++j) {
      if (hay[i + j] != nee[j]) { match = false; break; }
    }
    if (match) return true;
  }
  return false;
}

static bool any_naive(std::string_view line, const std::vector<std::string>& needles) {
  for (const auto& needle : needles) {
    if (contains_naive(line.data(), line.size(), needle.data(), needle.size())) return true;
  }
  return false;
}

static bool any_memmem(std::string_view line, const std::vector<std::string>& needles) {
  for (const auto& needle : needles) {
    if (memmem(line.data(), line.size(), needle.data(), needle.size()) != nullptr) return true;
  }
  return false;
}

// Teddy-style filter: for every needle keep its first two bytes. Scan the line
// 32 bytes at a time, testing whether any position matches any needle's first
// byte AND the following byte matches that needle's second byte. Only surviving
// positions are verified with a full comparison.
struct TeddyIndex {
  std::vector<__m256i> first;   // broadcast first byte per needle
  std::vector<__m256i> second;  // broadcast second byte per needle
  const std::vector<std::string>* needles;
};

static TeddyIndex build_teddy(const std::vector<std::string>& needles) {
  TeddyIndex index;
  index.needles = &needles;
  for (const auto& needle : needles) {
    index.first.push_back(_mm256_set1_epi8(needle[0]));
    index.second.push_back(_mm256_set1_epi8(needle.size() > 1 ? needle[1] : needle[0]));
  }
  return index;
}

static bool any_teddy(std::string_view line, const TeddyIndex& index) {
  const auto& needles = *index.needles;
  const size_t n = line.size();
  const char* data = line.data();
  if (n < 32) return any_memmem(line, needles);

  for (size_t offset = 0; offset + 32 <= n; offset += 31) {
    const __m256i block =
        _mm256_loadu_si256(reinterpret_cast<const __m256i*>(data + offset));
    const __m256i shifted =
        _mm256_loadu_si256(reinterpret_cast<const __m256i*>(data + offset + 1));
    for (size_t k = 0; k < needles.size(); ++k) {
      const __m256i hitFirst = _mm256_cmpeq_epi8(block, index.first[k]);
      const __m256i hitSecond = _mm256_cmpeq_epi8(shifted, index.second[k]);
      unsigned mask = static_cast<unsigned>(
          _mm256_movemask_epi8(_mm256_and_si256(hitFirst, hitSecond)));
      // The last lane's successor byte lies outside this block's 31 usable
      // positions; drop it so it is examined by the next iteration.
      mask &= 0x7fffffffu;
      while (mask) {
        const int bit = __builtin_ctz(mask);
        mask &= mask - 1;
        const size_t at = offset + static_cast<size_t>(bit);
        const auto& needle = needles[k];
        if (at + needle.size() <= n &&
            memcmp(data + at, needle.data(), needle.size()) == 0) {
          return true;
        }
      }
    }
  }
  // Tail: the final window that the strided loop could not cover.
  const size_t tailStart = n >= 64 ? n - 64 : 0;
  return any_memmem(std::string_view(data + tailStart, n - tailStart), needles);
}

int main(int argc, char** argv) {
  if (argc < 2) { fprintf(stderr, "usage: prefilter <corpus.jsonl>\n"); return 2; }
  std::ifstream input(argv[1], std::ios::binary);
  std::string blob((std::istreambuf_iterator<char>(input)), std::istreambuf_iterator<char>());

  std::vector<std::string_view> lines;
  size_t start = 0;
  for (size_t i = 0; i < blob.size(); ++i) {
    if (blob[i] == '\n') { lines.emplace_back(blob.data() + start, i - start); start = i + 1; }
  }
  size_t bytes = blob.size();
  printf("corpus: %zu lines, %.2f MiB, %zu needles\n", lines.size(),
         static_cast<double>(bytes) / (1024 * 1024), kNeedles.size());

  TeddyIndex teddy = build_teddy(kNeedles);

  struct Variant { const char* name; bool (*run)(std::string_view, const std::vector<std::string>&, const TeddyIndex&); };
  auto runNaive  = [](std::string_view l, const std::vector<std::string>& ns, const TeddyIndex&) { return any_naive(l, ns); };
  auto runMemmem = [](std::string_view l, const std::vector<std::string>& ns, const TeddyIndex&) { return any_memmem(l, ns); };
  auto runTeddy  = [](std::string_view l, const std::vector<std::string>&, const TeddyIndex& t) { return any_teddy(l, t); };

  const int reps = 5;
  size_t reference = 0;
  for (int variant = 0; variant < 3; ++variant) {
    const char* name = variant == 0 ? "scalar_naive" : variant == 1 ? "scalar_memmem" : "simd_teddy";
    double best = 1e18;
    size_t hits = 0;
    for (int r = 0; r < reps; ++r) {
      auto t0 = Clock::now();
      size_t count = 0;
      for (auto line : lines) {
        bool hit = variant == 0 ? runNaive(line, kNeedles, teddy)
                 : variant == 1 ? runMemmem(line, kNeedles, teddy)
                                : runTeddy(line, kNeedles, teddy);
        count += hit ? 1 : 0;
      }
      auto t1 = Clock::now();
      double seconds = std::chrono::duration<double>(t1 - t0).count();
      best = std::min(best, seconds);
      hits = count;
    }
    if (variant == 0) reference = hits;
    printf("%-14s %8.4f s  %8.1f MiB/s  hits=%zu%s\n", name, best,
           static_cast<double>(bytes) / (1024 * 1024) / best, hits,
           hits == reference ? "" : "  <-- MISMATCH");
  }
  return 0;
}
