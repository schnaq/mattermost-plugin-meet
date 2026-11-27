// Two-tone ring generated with Web Audio (no asset, no license). Fails silently when the
// browser blocks audio before a user gesture: the dialog still shows.
let ctx: AudioContext | undefined;
let timer: ReturnType<typeof setInterval> | undefined;

function beep(c: AudioContext, freq: number, at: number) {
    const osc = c.createOscillator();
    const gain = c.createGain();
    osc.frequency.value = freq;
    gain.gain.setValueAtTime(0.0001, at);
    gain.gain.exponentialRampToValueAtTime(0.2, at + 0.02);
    gain.gain.exponentialRampToValueAtTime(0.0001, at + 0.4);
    osc.connect(gain).connect(c.destination);
    osc.start(at);
    osc.stop(at + 0.45);
}

export function startRing() {
    if (timer) {
        return;
    }
    try {
        const c = ctx ?? new AudioContext();
        ctx = c;
        c.resume().catch(() => {});
        const pattern = () => {
            beep(c, 440, c.currentTime);
            beep(c, 480, c.currentTime + 0.5);
        };
        pattern();
        timer = setInterval(pattern, 2000);
    } catch {
        // no Web Audio or blocked: silent
    }
}

export function stopRing() {
    clearInterval(timer);
    timer = undefined;
    ctx?.suspend().catch(() => {});
}
