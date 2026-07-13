package net.sieganet.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import net.sieganet.app.R
import net.sieganet.app.ui.theme.Accent
import net.sieganet.app.ui.theme.AccentLight
import net.sieganet.app.ui.theme.Inter
import net.sieganet.app.ui.theme.Panel
import net.sieganet.app.ui.theme.TextMain
import net.sieganet.app.ui.theme.TextMuted

/**
 * Shown when connect is tapped without an active subscription: explains it's
 * unavailable, offers to renew, and — per the brief — a "Воспользоваться
 * кодом" link that opens code entry in the profile.
 */
@Composable
fun SubscriptionDialog(
    onRenew: () -> Unit,
    onUseCode: () -> Unit,
    onDismiss: () -> Unit,
) {
    Dialog(onDismissRequest = onDismiss) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .background(Panel, RoundedCornerShape(20.dp))
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Box(
                modifier = Modifier
                    .size(54.dp)
                    .background(Accent.copy(alpha = 0.14f), RoundedCornerShape(16.dp)),
                contentAlignment = Alignment.Center,
            ) {
                Icon(painterResource(R.drawable.ic_lock), contentDescription = null, tint = AccentLight, modifier = Modifier.size(26.dp))
            }
            Spacer(Modifier.height(14.dp))
            Text("Нужна активная подписка", fontFamily = Inter, fontWeight = FontWeight.Bold, fontSize = 17.sp, color = TextMain, textAlign = TextAlign.Center)
            Spacer(Modifier.height(8.dp))
            Text(
                "Подключение недоступно без активной подписки. Продлите доступ или воспользуйтесь кодом подписки.",
                fontFamily = Inter, fontSize = 13.sp, color = TextMuted, textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(20.dp))
            Button(
                onClick = onRenew,
                shape = RoundedCornerShape(12.dp),
                colors = ButtonDefaults.buttonColors(containerColor = Accent),
                modifier = Modifier.fillMaxWidth().height(48.dp),
            ) {
                Text("Продлить подписку", fontFamily = Inter, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            }
            TextButton(onClick = onUseCode) {
                Text(
                    "Воспользоваться кодом",
                    fontFamily = Inter, fontSize = 13.sp, color = AccentLight,
                    textDecoration = TextDecoration.Underline,
                )
            }
        }
    }
}
