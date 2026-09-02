# Measurement methodology

A manual test first warms WebSocket application RTT, then transfers parallel HTTP streams. The intended client coordinator increases 1→2→4→8 streams, samples every 250ms, excludes warm-up, and uses a median stable window rather than a peak. Directional work is capped at 15 seconds. Stability samples are one second apart only while visible; jitter is the mean absolute difference between consecutive RTTs from the last 30 samples. Results represent the deployed ingress route.
