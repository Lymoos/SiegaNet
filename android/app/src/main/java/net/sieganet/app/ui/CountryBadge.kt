package net.sieganet.app.ui

import androidx.annotation.DrawableRes
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.ColorMatrix
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import net.sieganet.app.R
import net.sieganet.app.api.isoFor
import net.sieganet.app.ui.theme.Mono
import net.sieganet.app.ui.theme.TextMain

/** country name -> flag drawable (rasterised flag-icons 4x3), 0 if unknown */
@DrawableRes
private fun flagRes(country: String): Int = when (isoFor(country).lowercase()) {
    "nl" -> R.drawable.flag_nl; "de" -> R.drawable.flag_de; "fi" -> R.drawable.flag_fi
    "se" -> R.drawable.flag_se; "gb" -> R.drawable.flag_gb; "fr" -> R.drawable.flag_fr
    "pl" -> R.drawable.flag_pl; "tr" -> R.drawable.flag_tr; "kz" -> R.drawable.flag_kz
    "ae" -> R.drawable.flag_ae; "sg" -> R.drawable.flag_sg; "jp" -> R.drawable.flag_jp
    "us" -> R.drawable.flag_us; "br" -> R.drawable.flag_br
    else -> 0
}

/**
 * Circular country flag. A real rectangular flag is cropped into a centred
 * circle (ContentScale.Crop) — it fills the badge and never overflows. The
 * ring (server load colour) hugs the circle. Falls back to the ISO code only
 * if a flag drawable is missing.
 */
@Composable
fun CountryBadge(
    country: String,
    size: Dp = 42.dp,
    ring: Color? = null,
) {
    val res = flagRes(country)
    val ringColor = ring ?: Color(0x3AA78BFA)

    Box(modifier = Modifier.size(size), contentAlignment = Alignment.Center) {
        if (res != 0) {
            Image(
                painter = painterResource(res),
                contentDescription = country,
                contentScale = ContentScale.Crop,
                // toned so bright flags don't pop out of the dark theme
                colorFilter = ColorFilter.colorMatrix(ColorMatrix().apply { setToSaturation(0.82f) }),
                modifier = Modifier.size(size).clip(CircleShape),
            )
            // darken + vignette overlay: seats the flag into the dark UI
            Box(
                modifier = Modifier
                    .size(size)
                    .clip(CircleShape)
                    .background(
                        Brush.radialGradient(
                            colors = listOf(Color(0x1F000000), Color(0x5C000000)),
                        ),
                    ),
            )
        } else {
            Box(
                modifier = Modifier
                    .size(size)
                    .clip(CircleShape)
                    .background(Brush.radialGradient(listOf(Color(0xFF2A2140), Color(0xFF17121F)))),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    isoFor(country),
                    fontFamily = Mono,
                    fontWeight = FontWeight.Medium,
                    fontSize = (size.value * 0.32f).sp,
                    color = TextMain,
                )
            }
        }
        // ring overlay on top of the flag edge
        Box(Modifier.size(size).border(2.dp, ringColor, CircleShape))
    }
}
