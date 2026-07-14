package net.sieganet.app.ui

import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay
import net.sieganet.app.R
import net.sieganet.app.api.Server
import net.sieganet.app.api.Status
import net.sieganet.app.api.VpnState
import net.sieganet.app.ui.theme.Accent
import net.sieganet.app.ui.theme.AccentDeep
import net.sieganet.app.ui.theme.AccentLight
import net.sieganet.app.ui.theme.Bg
import net.sieganet.app.ui.theme.Inter
import net.sieganet.app.ui.theme.Mono
import net.sieganet.app.ui.theme.Ok
import net.sieganet.app.ui.theme.Outfit
import net.sieganet.app.ui.theme.Panel
import net.sieganet.app.ui.theme.TextMain
import net.sieganet.app.ui.theme.TextMuted

/**
 * Approved phone design: status on top, one loud circular connect button with
 * a purple glow (pulsing while connecting), session timer under the button,
 * one-tap server selector at the bottom.
 */
@Composable
fun ConnectScreen(
    status: Status,
    selected: Server?,
    /** «подписка до …» / «Без подписки» line under the state label */
    subscriptionNote: String?,
    hasSub: Boolean,
    accountInitial: String,
    onConnectClick: () -> Unit,
    onDisconnectClick: () -> Unit,
    onServerClick: () -> Unit,
    onProfileClick: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(Bg)
            .padding(horizontal = 24.dp, vertical = 16.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        // ---- top bar --------------------------------------------------------
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                "SiegaNet",
                fontFamily = Outfit,
                fontWeight = FontWeight.Bold,
                fontSize = 20.sp,
                color = TextMain,
            )
            Spacer(Modifier.weight(1f))
            ProfileButton(initial = accountInitial, pro = hasSub, onClick = onProfileClick)
        }

        Spacer(Modifier.height(28.dp))

        // ---- status ---------------------------------------------------------
        val (stateText, stateColor) = when (status.state) {
            VpnState.CONNECTED -> "ЗАЩИЩЕНО" to Ok
            VpnState.CONNECTING -> "ПОДКЛЮЧЕНИЕ…" to AccentLight
            VpnState.DISCONNECTED -> "НЕ ПОДКЛЮЧЕНО" to TextMuted
        }
        Text(
            stateText,
            fontFamily = Mono,
            fontSize = 13.sp,
            letterSpacing = 3.sp,
            color = stateColor,
        )
        Spacer(Modifier.height(6.dp))
        Text(
            subscriptionNote ?: "",
            fontFamily = Inter,
            fontSize = 12.sp,
            color = TextMuted,
            textAlign = TextAlign.Center,
        )

        Spacer(Modifier.weight(1f))

        // ---- the one loud element -------------------------------------------
        PowerButton(
            state = status.state,
            locked = !hasSub && status.state == VpnState.DISCONNECTED,
            onClick = {
                if (status.state == VpnState.DISCONNECTED) onConnectClick()
                else onDisconnectClick()
            },
        )

        Spacer(Modifier.height(30.dp))

        // ---- session timer + traffic ----------------------------------------
        var nowSec by remember { mutableLongStateOf(System.currentTimeMillis() / 1000) }
        LaunchedEffect(status.state) {
            while (status.state == VpnState.CONNECTED) {
                nowSec = System.currentTimeMillis() / 1000
                delay(1000)
            }
        }
        if (status.state == VpnState.CONNECTED && status.sinceUnix != null) {
            Text(
                formatDuration(status.sinceUnix, nowSec),
                fontFamily = Mono,
                fontSize = 30.sp,
                color = TextMain,
            )
            Spacer(Modifier.height(6.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(18.dp)) {
                Text(
                    "↑ ${formatBytes(status.upBytes)}",
                    fontFamily = Mono, fontSize = 13.sp, color = AccentLight,
                )
                Text(
                    "↓ ${formatBytes(status.downBytes)}",
                    fontFamily = Mono, fontSize = 13.sp, color = Ok,
                )
            }
        } else {
            Text(
                if (status.state == VpnState.CONNECTING) "устанавливаем туннель…"
                else "нажмите, чтобы подключиться",
                fontFamily = Inter,
                fontSize = 13.sp,
                color = TextMuted,
            )
        }

        Spacer(Modifier.weight(1f))

        // ---- server selector -------------------------------------------------
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .background(Panel, RoundedCornerShape(16.dp))
                .clickable(onClick = onServerClick)
                .padding(horizontal = 16.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (selected != null) {
                CountryBadge(selected.country, size = 38.dp, ring = loadColor(selected.loadPct))
            } else {
                CountryBadge("", size = 38.dp)
            }
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    selected?.country ?: "Выбрать сервер",
                    fontFamily = Inter,
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 15.sp,
                    color = TextMain,
                )
                if (selected != null) {
                    Text(
                        selected.city,
                        fontFamily = Inter, fontSize = 12.sp, color = TextMuted,
                    )
                }
            }
            if (selected != null) {
                Text(
                    "${selected.pingMs} ms",
                    fontFamily = Mono, fontSize = 13.sp,
                    color = if (selected.pingMs < 80) Ok else AccentLight,
                )
            }
            Spacer(Modifier.width(10.dp))
            Text("›", fontSize = 22.sp, color = TextMuted)
        }
    }
}

/** diameter of the inner clickable circle */
private val BUTTON_SIZE = 172.dp
/** gap between the button's border and the connecting arc */
private val ARC_GAP = 7.dp
private val ARC_STROKE = 3.dp

@Composable
private fun PowerButton(state: VpnState, locked: Boolean, onClick: () -> Unit) {
    val anim = rememberInfiniteTransition(label = "power")

    // connecting: a thin violet arc wraps around the ring — the head circles
    // steadily while the sweep breathes, so the handshake reads as motion
    // along the ring instead of the old expanding pulse
    val arcRotation by anim.animateFloat(
        initialValue = 0f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(
            animation = tween(1500, easing = LinearEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "arcRotation",
    )
    val arcSweep by anim.animateFloat(
        initialValue = 40f,
        targetValue = 290f,
        animationSpec = infiniteRepeatable(
            animation = tween(1000, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "arcSweep",
    )
    // slow breathing for the connected glow (calm, mirrors the arc's motion)
    val breathe by anim.animateFloat(
        initialValue = 0.85f,
        targetValue = 1.12f,
        animationSpec = infiniteRepeatable(
            animation = tween(2600, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "breathe",
    )

    val glowColor = when (state) {
        VpnState.CONNECTED -> Ok
        else -> Accent
    }
    // connected: steady strong glow; connecting/disconnected: quiet glow
    // (while connecting the arc is the signal, the glow stays calm)
    val glowAlpha = when (state) {
        VpnState.CONNECTING -> 0.32f
        VpnState.CONNECTED -> 0.45f
        VpnState.DISCONNECTED -> 0.30f
    }
    val glowScale = when (state) {
        VpnState.CONNECTING -> 0.95f
        VpnState.CONNECTED -> breathe
        VpnState.DISCONNECTED -> 0.9f
    }

    Box(
        modifier = Modifier
            .size(230.dp)
            .drawBehind {
                drawCircle(
                    brush = Brush.radialGradient(
                        colors = listOf(
                            glowColor.copy(alpha = glowAlpha),
                            glowColor.copy(alpha = glowAlpha * 0.35f),
                            Color.Transparent,
                        ),
                        center = Offset(size.width / 2, size.height / 2),
                        radius = size.minDimension / 2 * glowScale * 1.15f,
                    ),
                    radius = size.minDimension / 2 * glowScale * 1.15f,
                )
                if (state == VpnState.CONNECTING) {
                    val arcRadius = BUTTON_SIZE.toPx() / 2 + ARC_GAP.toPx()
                    drawArc(
                        color = AccentLight,
                        startAngle = arcRotation - 90f,
                        sweepAngle = arcSweep,
                        useCenter = false,
                        topLeft = Offset(
                            size.width / 2 - arcRadius,
                            size.height / 2 - arcRadius,
                        ),
                        size = Size(arcRadius * 2, arcRadius * 2),
                        style = Stroke(ARC_STROKE.toPx(), cap = StrokeCap.Round),
                    )
                }
            },
        contentAlignment = Alignment.Center,
    ) {
        Column(
            modifier = Modifier
                .size(BUTTON_SIZE)
                .background(
                    Brush.linearGradient(listOf(Panel, Bg)),
                    CircleShape,
                )
                .border(
                    2.dp,
                    Brush.linearGradient(
                        listOf(
                            if (state == VpnState.CONNECTED) Ok else AccentLight,
                            AccentDeep,
                        ),
                    ),
                    CircleShape,
                )
                .clickable(onClick = onClick),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Icon(
                painterResource(if (locked) R.drawable.ic_lock else R.drawable.ic_tile),
                contentDescription = null,
                tint = if (state == VpnState.CONNECTED) Ok else AccentLight,
                modifier = Modifier.size(if (locked) 34.dp else 44.dp),
            )
            Spacer(Modifier.height(10.dp))
            Text(
                when (state) {
                    VpnState.CONNECTED -> "Отключить"
                    VpnState.CONNECTING -> "Отмена"
                    VpnState.DISCONNECTED -> "Подключиться"
                },
                fontFamily = Inter,
                fontWeight = FontWeight.SemiBold,
                fontSize = 16.sp,
                color = TextMain,
            )
        }
    }
}

/** avatar chip in the top bar; a green PRO ring when subscribed */
@Composable
private fun ProfileButton(initial: String, pro: Boolean, onClick: () -> Unit) {
    Box(
        modifier = Modifier
            .size(38.dp)
            .then(
                if (pro) Modifier.border(2.dp, Ok, CircleShape) else Modifier,
            )
            .padding(if (pro) 3.dp else 0.dp)
            .background(Brush.linearGradient(listOf(Accent, AccentDeep)), CircleShape)
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            initial,
            fontFamily = Inter,
            fontWeight = FontWeight.Bold,
            fontSize = 15.sp,
            color = Color.White,
        )
    }
}
