@file:OptIn(androidx.compose.ui.text.ExperimentalTextApi::class)

package net.sieganet.app.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import net.sieganet.app.R

// ---- design tokens (design/tokens.json — keep in sync) ---------------------

val Bg = Color(0xFF0B0910)
val Panel = Color(0xFF1B1626)
val Accent = Color(0xFF8B5CF6)
val AccentLight = Color(0xFFA78BFA)
val AccentDeep = Color(0xFF5B21B6)
val Ok = Color(0xFF5EE0A0)
val TextMain = Color(0xFFECE9F5)
val TextMuted = Color(0xFF928AA6)

// ---- fonts (bundled variable TTFs) -----------------------------------------

private fun variable(res: Int, weight: FontWeight) = Font(
    resId = res,
    weight = weight,
    variationSettings = FontVariation.Settings(FontVariation.weight(weight.weight)),
)

/** headings, country names, buttons; no Cyrillic — Compose falls back to Inter/system for it */
val Outfit = FontFamily(
    variable(R.font.outfit, FontWeight.Medium),
    variable(R.font.outfit, FontWeight.SemiBold),
    variable(R.font.outfit, FontWeight.Bold),
)

/** body text (has Cyrillic) */
val Inter = FontFamily(
    variable(R.font.inter, FontWeight.Normal),
    variable(R.font.inter, FontWeight.Medium),
    variable(R.font.inter, FontWeight.SemiBold),
)

/** ping, data, labels */
val Mono = FontFamily(
    variable(R.font.jetbrains_mono, FontWeight.Normal),
    variable(R.font.jetbrains_mono, FontWeight.Medium),
)

private val Colors = darkColorScheme(
    primary = Accent,
    onPrimary = Color.White,
    secondary = AccentLight,
    tertiary = AccentDeep,
    background = Bg,
    onBackground = TextMain,
    surface = Panel,
    onSurface = TextMain,
    surfaceVariant = Panel,
    onSurfaceVariant = TextMuted,
    outline = AccentLight.copy(alpha = 0.22f),
)

@Composable
fun SiegaNetTheme(content: @Composable () -> Unit) {
    // the design is committed to near-black; light system theme changes nothing
    isSystemInDarkTheme()
    MaterialTheme(
        colorScheme = Colors,
        typography = androidx.compose.material3.Typography(
            headlineMedium = TextStyle(
                fontFamily = Inter, fontWeight = FontWeight.SemiBold, fontSize = 26.sp,
            ),
            titleMedium = TextStyle(
                fontFamily = Inter, fontWeight = FontWeight.SemiBold, fontSize = 17.sp,
            ),
            bodyMedium = TextStyle(
                fontFamily = Inter, fontWeight = FontWeight.Normal, fontSize = 14.sp,
            ),
            labelMedium = TextStyle(
                fontFamily = Mono, fontWeight = FontWeight.Medium, fontSize = 12.sp,
            ),
        ),
        content = content,
    )
}
