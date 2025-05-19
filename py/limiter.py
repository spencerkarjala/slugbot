#!/usr/bin/env python3
"""
Brick-wall limiter with true envelope follower, attack/release smoothing, 
makeup gain, stereo support, and plotting.
Usage:
  python limiter_full.py \
    --input  input.wav \
    --output output.wav \
    [--threshold 0.1] [--attack_ms 5] [--release_ms 50]
"""
import argparse
import numpy as np
from scipy.io import wavfile
from scipy.ndimage import maximum_filter1d
import matplotlib.pyplot as plt

def limiter(audio, sr, threshold, attack_ms, release_ms):
    # if given a 1D array, make it 2D; eg. size (12) becomes size (12, 1)
    if audio.ndim == 1:
        audio = audio[:, np.newaxis]
    n_samples, n_channels = audio.shape

    for n in range(1, n_samples):
        val = audio[n][0]
        if abs(val) > 1.0:
            print(f"got out of bounds input value: {val} at index {n}")

    # time constants in seconds
    attack_tc  = attack_ms  / 1000.0
    release_tc = release_ms / 1000.0

    # filter coefficients
    alpha_a = np.exp(-1.0 / (sr * attack_tc))
    alpha_r = np.exp(-1.0 / (sr * release_tc))

    # rectify/abs the input signal
    rectified = np.abs(audio)

    # 1) envelope follower (per-sample attack/release)
    env      = np.zeros((n_samples, n_channels), dtype=np.float32)
    env[0]   = np.abs(audio[0])
    for n in range(1, n_samples):
        x = rectified[n, :]
        env[n] = np.where(x > env[n-1],
                          alpha_a   * env[n-1] + (1 - alpha_a)   * x,
                          alpha_r   * env[n-1] + (1 - alpha_r)   * x)

    # 2) raw gain to never exceed threshold
    gain_raw = np.minimum(1.0, threshold / (env + 1e-9))

    # 3) smooth gain with release only
    gain     = np.zeros_like(gain_raw)
    gain[0]  = gain_raw[0]
    for n in range(1, n_samples):
        gain[n] = np.maximum(gain_raw[n], gain[n-1] * alpha_r)

    # 4) apply gain
    limited  = audio * gain

    # 5) makeup so threshold→1.0
    limited *= (1.0 / threshold)

    # 6) hard clip any peaks that have snuck through
    limited = np.clip(limited, -1.0, 1.0)

    return limited, env, gain_raw, gain

def main():
    p = argparse.ArgumentParser(description="Brick-wall envelope-follower limiter")
    p.add_argument('-i','--input',     required=True, help="Input WAV file")
    p.add_argument('-o','--output',    required=True, help="Output WAV file")
    p.add_argument('-t','--threshold', type=float, default=0.5,  help="Limiter threshold (0-1)")
    p.add_argument(    '--attack_ms',  type=float, default=5.0,  help="Attack time constant (ms)")
    p.add_argument(    '--release_ms', type=float, default=50.0, help="Release time constant (ms)")
    args = p.parse_args()

    # load
    sr, data = wavfile.read(args.input)
    orig_dtype = data.dtype
    is_int     = np.issubdtype(orig_dtype, np.integer)
    if is_int:
        max_val = np.iinfo(orig_dtype).max
        audio   = data.astype(np.float32) / max_val
    else:
        audio   = data.astype(np.float32)

    # limit
    limited, env, gain_raw, gain = limiter(
        audio, sr,
        args.threshold,
        args.attack_ms,
        args.release_ms
    )

    # time axis
    times = np.arange(limited.shape[0]) / sr

    # # plot envelope & gain (first channel)
    # plt.figure(figsize=(10,4))
    # plt.plot(times, env[:,0],    label="Envelope")
    # plt.plot(times, gain_raw[:,0],label="Raw Gain")
    # plt.plot(times, gain[:,0],    label="Smoothed Gain")
    # plt.title("Envelope & Gain (Ch 1)")
    # plt.xlabel("Time [s]")
    # plt.ylabel("Level")
    # plt.legend()
    # plt.tight_layout()
    # plt.show()

    # # plot waveforms (first channel)
    # plt.figure(figsize=(10,4))
    # plt.subplot(2,1,1)
    # plt.plot(times, audio[:,0],  label="Original")
    # plt.ylim(-1.5, 1.5)
    # plt.title("Original Waveform (Ch 1)")
    # plt.subplot(2,1,2)
    # plt.plot(times, limited[:,0],label="Limited")
    # plt.ylim(-1.5, 1.5)
    # plt.title("Limited Waveform (Ch 1)")
    # plt.tight_layout()
    # plt.show()

    # write output
    if is_int:
        out = (limited * max_val).astype(orig_dtype)
    else:
        out = limited
    # flatten mono
    if out.ndim==2 and out.shape[1]==1:
        out = out[:,0]
    wavfile.write(args.output, sr, out)

if __name__=="__main__":
    main()
