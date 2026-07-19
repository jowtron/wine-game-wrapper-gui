# Test fixtures

- `comt.arcv` — a small ARCV ("EDI Install archive") sample used by the
  decoder tests in `arcv_test.go`. It is `COMT.$00` from `wramp12.zip`
  (WinRamp 1.2, a 1994 freely-distributed shareware package, obtained via
  discmaster.textfiles.com), containing `comt.wri` (3,072 bytes, a
  Microsoft Write document). Chosen because it is the smallest real ARCV
  file we have with a known-good CRC.
