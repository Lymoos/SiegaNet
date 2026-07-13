package net.sieganet.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import net.sieganet.app.api.Server
import net.sieganet.app.ui.theme.AccentLight
import net.sieganet.app.ui.theme.Inter
import net.sieganet.app.ui.theme.Mono
import net.sieganet.app.ui.theme.Ok
import net.sieganet.app.ui.theme.Panel
import net.sieganet.app.ui.theme.TextMain
import net.sieganet.app.ui.theme.TextMuted

private data class SheetRow(val server: Server, val ring: Color)

/**
 * Bottom sheet with the server list: страна, город, пинг — one tap selects.
 * The ring around each country badge encodes server load with the same
 * green→orange→red ramp as the desktop map pins (see [loadColor]).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ServerSheet(
    servers: List<Server>,
    selectedId: String?,
    onPick: (Server) -> Unit,
    onDismiss: () -> Unit,
) {
    // skipPartiallyExpanded: one anchor instead of two — the sheet opens in
    // a single settle animation, noticeably faster on slow emulators
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    // sort once per server-list change, not on every recomposition; ring
    // colours are precomputed so rows do zero work while scrolling
    val rows = remember(servers) {
        servers.sortedBy { it.pingMs }.map { SheetRow(it, loadColor(it.loadPct)) }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = Panel,
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 24.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                "Серверы",
                fontFamily = Mono,
                fontSize = 12.sp,
                letterSpacing = 2.sp,
                color = TextMuted,
            )
            Spacer(Modifier.weight(1f))
            LoadLegend()
        }
        LazyColumn(modifier = Modifier.padding(bottom = 24.dp)) {
            items(rows, key = { it.server.id }) { (s, ring) ->
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { onPick(s) }
                        .padding(horizontal = 24.dp, vertical = 11.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    // country badge in a ring: ring colour = load (as on the map)
                    CountryBadge(s.country, size = 42.dp, ring = ring)
                    Spacer(Modifier.width(14.dp))
                    Column(Modifier.weight(1f)) {
                        Text(
                            s.country,
                            fontFamily = Inter,
                            fontWeight = FontWeight.SemiBold,
                            fontSize = 15.sp,
                            color = TextMain,
                        )
                        Text(
                            s.city,
                            fontFamily = Inter,
                            fontSize = 12.sp,
                            color = TextMuted,
                        )
                    }
                    Text(
                        "${s.pingMs} ms",
                        fontFamily = Mono,
                        fontSize = 13.sp,
                        color = when {
                            s.pingMs < 80 -> Ok
                            s.pingMs < 160 -> AccentLight
                            else -> TextMuted
                        },
                    )
                    if (s.id == selectedId) {
                        Spacer(Modifier.width(12.dp))
                        Text("✓", color = AccentLight, fontSize = 16.sp)
                    }
                }
            }
            item { Spacer(Modifier.height(8.dp)) }
        }
    }
}

/** «нагрузка ● ● ●» — decodes the flag rings, mirrors the desktop map legend */
@Composable
private fun LoadLegend() {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(
            "нагрузка",
            fontFamily = Mono,
            fontSize = 10.sp,
            letterSpacing = 1.sp,
            color = TextMuted.copy(alpha = 0.8f),
        )
        for (pct in listOf(15, 55, 90)) {
            Spacer(Modifier.width(6.dp))
            Box(
                Modifier
                    .size(8.dp)
                    .background(loadColor(pct), CircleShape),
            )
        }
    }
}
