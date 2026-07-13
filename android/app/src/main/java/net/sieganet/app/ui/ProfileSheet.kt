package net.sieganet.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import net.sieganet.app.account.AccountStore
import net.sieganet.app.ui.theme.Accent
import net.sieganet.app.ui.theme.AccentDeep
import net.sieganet.app.ui.theme.AccentLight
import net.sieganet.app.ui.theme.Inter
import net.sieganet.app.ui.theme.Mono
import net.sieganet.app.ui.theme.Ok
import net.sieganet.app.ui.theme.Panel
import net.sieganet.app.ui.theme.TextMain
import net.sieganet.app.ui.theme.TextMuted
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Account profile bottom sheet: email, subscription status, subscription-code
 * redemption (moved here from the old pre-app gate), logout. This sheet is
 * where a code is entered — the "воспользоваться кодом" flow opens it.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProfileSheet(
    account: AccountStore.Account,
    busy: Boolean,
    error: String?,
    onRedeem: (String) -> Unit,
    onLogout: () -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var code by remember { mutableStateOf("") }

    val active = account.hasActiveSubscription

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = Panel,
    ) {
        Column(Modifier.padding(horizontal = 24.dp).padding(bottom = 28.dp)) {
            // header: avatar + email + plan dot
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(
                    modifier = Modifier
                        .size(52.dp)
                        .background(Brush.linearGradient(listOf(Accent, AccentDeep)), CircleShape),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        account.email.take(1).uppercase(),
                        fontFamily = Inter, fontWeight = FontWeight.Bold, fontSize = 22.sp, color = Color.White,
                    )
                }
                Spacer(Modifier.width(14.dp))
                Column {
                    Text(account.email, fontFamily = Inter, fontWeight = FontWeight.SemiBold, fontSize = 15.sp, color = TextMain)
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier
                                .size(8.dp)
                                .background(if (active) Ok else TextMuted, CircleShape),
                        )
                        Spacer(Modifier.width(7.dp))
                        Text(
                            if (active) "Подписка ${account.plan}" else "Без подписки",
                            fontFamily = Inter, fontSize = 12.5.sp, color = TextMuted,
                        )
                    }
                }
            }

            Spacer(Modifier.height(18.dp))

            // subscription card
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .background(
                        if (active) Brush.linearGradient(listOf(Ok.copy(alpha = 0.14f), Panel))
                        else Brush.linearGradient(listOf(Color(0xFF221A31), Color(0xFF221A31))),
                        RoundedCornerShape(12.dp),
                    )
                    .padding(14.dp),
            ) {
                Text(
                    if (active) "Подписка активна" else "Подписка не активна",
                    fontFamily = Inter, fontWeight = FontWeight.SemiBold, fontSize = 14.sp, color = TextMain,
                )
                Spacer(Modifier.height(4.dp))
                Text(
                    if (active) "действует до " + SimpleDateFormat("d MMMM yyyy", Locale("ru"))
                        .format(Date(account.validUntilUnix * 1000))
                    else "Введите код подписки ниже или продлите доступ.",
                    fontFamily = Inter, fontSize = 12.5.sp, color = TextMuted,
                )
            }

            Spacer(Modifier.height(20.dp))

            Text("КОД ПОДПИСКИ", fontFamily = Mono, fontSize = 10.sp, letterSpacing = 2.sp, color = TextMuted)
            Spacer(Modifier.height(8.dp))
            OutlinedTextField(
                value = code,
                onValueChange = { code = it },
                enabled = !busy,
                singleLine = true,
                placeholder = { Text("XXXX-XXXX-XXXX-XXXX", fontFamily = Mono, fontSize = 14.sp, color = TextMuted.copy(alpha = 0.5f)) },
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                shape = RoundedCornerShape(12.dp),
                colors = OutlinedTextFieldDefaults.colors(
                    focusedBorderColor = Accent,
                    unfocusedBorderColor = AccentLight.copy(alpha = 0.25f),
                    cursorColor = AccentLight,
                    focusedTextColor = TextMain,
                    unfocusedTextColor = TextMain,
                ),
                modifier = Modifier.fillMaxWidth(),
            )

            if (error != null) {
                Spacer(Modifier.height(8.dp))
                Text(error, fontFamily = Inter, fontSize = 12.sp, color = Color(0xFFE08484))
            }

            Spacer(Modifier.height(12.dp))
            Button(
                onClick = { onRedeem(code) },
                enabled = !busy && code.trim().isNotEmpty(),
                shape = RoundedCornerShape(12.dp),
                colors = ButtonDefaults.buttonColors(containerColor = Accent),
                modifier = Modifier.fillMaxWidth().height(48.dp),
            ) {
                Text(if (busy) "Проверяем…" else "Применить код", fontFamily = Inter, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            }

            Spacer(Modifier.height(10.dp))
            OutlinedButton(
                onClick = onLogout,
                shape = RoundedCornerShape(12.dp),
                modifier = Modifier.fillMaxWidth().height(48.dp),
            ) {
                Text("Выйти из аккаунта", fontFamily = Inter, fontSize = 14.sp, color = TextMuted)
            }
        }
    }
}
