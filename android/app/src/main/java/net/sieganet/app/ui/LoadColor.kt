package net.sieganet.app.ui

import androidx.compose.ui.graphics.Color

/**
 * Load -> ring colour, the exact same encoding as the desktop map pins
 * (desktop/src/lib/flags.ts loadColor): green (free) → orange (busy) → red
 * (loaded), smooth over load_pct. Hue 145 (theme green) → 38 → 4.
 */
fun loadColor(loadPct: Int): Color {
    val t = loadPct.coerceIn(0, 100).toFloat()
    val hue = if (t <= 50f) 145f - (145f - 38f) * t / 50f
    else 38f - (38f - 4f) * (t - 50f) / 50f
    return Color.hsl(hue, 0.62f, 0.56f)
}
