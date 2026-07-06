package net.sieganet.app.vpn

import android.content.Intent
import android.net.VpnService
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import androidx.core.content.ContextCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import net.sieganet.app.MainActivity
import net.sieganet.app.R
import net.sieganet.app.api.VpnState

/**
 * Quick-settings tile: one-tap connect (to the selected/last server) or
 * disconnect. If the VPN consent dialog has not been granted yet the tile
 * opens the app, because consent can only be requested from an activity.
 */
class SiegaTileService : TileService() {

    private var scope: CoroutineScope? = null
    private var watch: Job? = null

    override fun onStartListening() {
        val s = CoroutineScope(SupervisorJob() + Dispatchers.Main)
        scope = s
        watch = s.launch {
            ConnectionRepository.status.collect { status ->
                qsTile?.apply {
                    state = when (status.state) {
                        VpnState.CONNECTED -> Tile.STATE_ACTIVE
                        VpnState.CONNECTING -> Tile.STATE_UNAVAILABLE
                        VpnState.DISCONNECTED -> Tile.STATE_INACTIVE
                    }
                    if (android.os.Build.VERSION.SDK_INT >= 29) {
                        subtitle = when (status.state) {
                            VpnState.CONNECTED -> getString(R.string.state_connected)
                            VpnState.CONNECTING -> getString(R.string.state_connecting)
                            VpnState.DISCONNECTED -> getString(R.string.state_disconnected)
                        }
                    }
                    updateTile()
                }
            }
        }
    }

    override fun onStopListening() {
        watch?.cancel()
        scope?.cancel()
        scope = null
    }

    override fun onClick() {
        val status = ConnectionRepository.status.value
        if (status.state != VpnState.DISCONNECTED) {
            ContextCompat.startForegroundService(
                this,
                SiegaVpnService.disconnectIntent(this),
            )
            return
        }

        if (VpnService.prepare(this) != null) {
            // consent missing — must go through the activity
            val intent = Intent(this, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            if (android.os.Build.VERSION.SDK_INT >= 34) {
                startActivityAndCollapse(
                    android.app.PendingIntent.getActivity(
                        this, 0, intent,
                        android.app.PendingIntent.FLAG_IMMUTABLE,
                    ),
                )
            } else {
                @Suppress("DEPRECATION", "StartActivityAndCollapseDeprecated")
                startActivityAndCollapse(intent)
            }
            return
        }

        val target = ConnectionRepository.selected.value ?: return
        ContextCompat.startForegroundService(
            this,
            SiegaVpnService.connectIntent(this, target.id),
        )
    }
}
