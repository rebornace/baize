/**
 * Streaming text reveal: paint the server snapshot as soon as it arrives.
 * Never pace behind the model — artificial typewriter lag was slower than
 * deepseek-flash and hurt perceived latency.
 *
 * `active` is kept for callers (caret / plain vs markdown); it does not delay
 * the visible string.
 */
export function useSmoothStreamText(target: string, _active: boolean): string {
  return target
}
