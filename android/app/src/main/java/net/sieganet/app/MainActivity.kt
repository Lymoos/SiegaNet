package net.sieganet.app

import android.net.VpnService
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import net.sieganet.app.ui.ActivationScreen
import net.sieganet.app.ui.ConnectScreen
import net.sieganet.app.ui.ServerSheet
import net.sieganet.app.ui.theme.SiegaNetTheme
import net.sieganet.app.vm.VpnViewModel
import net.sieganet.app.vpn.SiegaVpnService
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class MainActivity : ComponentActivity() {

    private val vm: VpnViewModel by viewModels()

    /** server chosen when the VPN consent dialog was launched */
    private var pendingServerId: String? = null

    private val vpnConsent = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        if (result.resultCode == RESULT_OK) {
            pendingServerId?.let(::startTunnel)
        }
        pendingServerId = null
    }

    private val notifPermission = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { /* notification is nice-to-have; VPN works either way */ }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        if (Build.VERSION.SDK_INT >= 33) {
            notifPermission.launch(android.Manifest.permission.POST_NOTIFICATIONS)
        }

        setContent {
            SiegaNetTheme {
                val activated by vm.activated.collectAsState()

                if (!activated) {
                    // subscription gate: no valid key — no VPN
                    val busy by vm.activationBusy.collectAsState()
                    val error by vm.activationError.collectAsState()
                    ActivationScreen(
                        busy = busy,
                        error = error,
                        onActivate = vm::activate,
                    )
                    return@SiegaNetTheme
                }

                val status by vm.status.collectAsState()
                val servers by vm.servers.collectAsState()
                val selected by vm.selected.collectAsState()
                var sheetOpen by remember { mutableStateOf(false) }

                ConnectScreen(
                    status = status,
                    selected = selected,
                    subscriptionNote = vm.validUntil?.let {
                        "подписка до " + SimpleDateFormat("dd.MM.yyyy", Locale.getDefault())
                            .format(Date(it * 1000))
                    },
                    onConnectClick = { selected?.let { connect(it.id) } },
                    onDisconnectClick = ::disconnect,
                    onServerClick = { sheetOpen = true },
                )

                if (sheetOpen) {
                    ServerSheet(
                        servers = servers,
                        selectedId = selected?.id,
                        onPick = {
                            vm.select(it)
                            sheetOpen = false
                        },
                        onDismiss = { sheetOpen = false },
                    )
                }
            }
        }
    }

    // ---- actions -------------------------------------------------------------

    private fun connect(serverId: String) {
        val consent = VpnService.prepare(this)
        if (consent != null) {
            pendingServerId = serverId
            vpnConsent.launch(consent)
        } else {
            startTunnel(serverId)
        }
    }

    private fun startTunnel(serverId: String) {
        ContextCompat.startForegroundService(
            this,
            SiegaVpnService.connectIntent(this, serverId),
        )
    }

    private fun disconnect() {
        ContextCompat.startForegroundService(
            this,
            SiegaVpnService.disconnectIntent(this),
        )
    }
}
