package net.sieganet.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import net.sieganet.app.api.isoFor
import net.sieganet.app.ui.theme.Mono
import net.sieganet.app.ui.theme.TextMain

/**
 * Replaces the emoji flags (which looked cheap and rendered inconsistently)
 * with a circular chip: 2-letter ISO code in mono on a subtle dark gradient.
 * The ring colour encodes server load — same green→orange→red as the desktop
 * map pins and sidebar (see [loadColor]).
 */
@Composable
fun CountryBadge(
    country: String,
    size: Dp = 42.dp,
    ring: Color? = null,
) {
    Box(
        modifier = Modifier
            .size(size)
            .background(
                Brush.radialGradient(
                    colors = listOf(Color(0xFF2A2140), Color(0xFF17121F)),
                    center = Offset(x = size.value * 0.9f, y = size.value * 0.7f),
                    radius = size.value * 2.2f,
                ),
                CircleShape,
            )
            .border(
                if (ring != null) 2.dp else 1.dp,
                ring ?: Color(0x3AA78BFA),
                CircleShape,
            ),
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
