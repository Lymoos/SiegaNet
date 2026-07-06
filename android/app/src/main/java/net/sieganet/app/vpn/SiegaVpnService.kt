package net.sieganet.app.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.ParcelFileDescriptor
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import net.sieganet.app.MainActivity
import net.sieganet.app.R
import net.sieganet.app.activation.ActivationStore
import net.sieganet.app.api.MockServers
import net.sieganet.app.api.Status
import net.sieganet.app.api.VpnState
import net.sieganet.app.core.CoreRegistry
import org.json.JSONObject

/**
 * Owns the TUN device and the foreground lifecycle — nothing else.
 *
 * connect:  establish() the TUN, detach the fd, hand it to the Go core
 *           (SiegaCore.start), then keep polling core.statusJson() into
 *           [ConnectionRepository] and mirror it in the notification.
 * disconnect: core.stop(), close the TUN, drop foreground.
 *
 * The tunnel itself (QUIC, auth, crypto, routing of payload) is entirely
 * inside the core.
 */
class SiegaVpnService : VpnService() {

    companion object {
        const val ACTION_CONNECT = "net.sieganet.app.CONNECT"
        const val ACTION_DISCONNECT = "net.sieganet.app.DISCONNECT"
        const val EXTRA_SERVER_ID = "server_id"

        private const val CHANNEL_ID = "vpn_status"
        private const val NOTIF_ID = 1

        /**
         * While the mock core is in place the device must keep real
         * connectivity, so only the inner subnet is routed into the TUN.
         * Flip to true when the gomobile core lands — then 0.0.0.0/0 goes
         * through the tunnel (full-tunnel + forced DNS, as on desktop).
         */
        private const val FULL_TUNNEL = false

        fun connectIntent(ctx: Context, serverId: String): Intent =
            Intent(ctx, SiegaVpnService::class.java)
                .setAction(ACTION_CONNECT)
                .putExtra(EXTRA_SERVER_ID, serverId)

        fun disconnectIntent(ctx: Context): Intent =
            Intent(ctx, SiegaVpnService::class.java).setAction(ACTION_DISCONNECT)
    }

    private var tun: ParcelFileDescriptor? = null
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var pollJob: Job? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_CONNECT -> {
                val serverId = intent.getStringExtra(EXTRA_SERVER_ID) ?: return START_NOT_STICKY
                startForeground(NOTIF_ID, buildNotification(getString(R.string.state_connecting)))
                connect(serverId)
            }
            ACTION_DISCONNECT -> disconnect()
        }
        return START_NOT_STICKY
    }

    private fun connect(serverId: String) {
        // subscription gate, second line of defence (the UI already blocks)
        if (!ActivationStore.isActivated(this)) {
            ConnectionRepository.push(Status())
            stopSelfCleanly()
            return
        }

        // one active tunnel at a time; switching server re-establishes
        teardownTunnel()

        val server = MockServers.byId(serverId)
        val builder = Builder()
            .setSession("SiegaNet")
            .setMtu(1280)
            .addAddress("10.77.0.2", 32)
            .addDnsServer("10.77.0.1")

        if (FULL_TUNNEL) {
            builder.addRoute("0.0.0.0", 0)
        } else {
            // mock phase: route only the inner subnet so the phone keeps
            // internet while the fake tunnel is "up"
            builder.addRoute("10.77.0.0", 24)
        }

        val pfd = builder.establish()
        if (pfd == null) {
            // VPN permission got revoked between prepare() and here
            ConnectionRepository.push(Status())
            stopSelfCleanly()
            return
        }
        tun = pfd

        // config for the core: chosen server + the activation key (the
        // backend resolves the key to peer credentials — subscription model,
        // no per-device config files). detachFd(): the core owns the fd from
        // here until stop().
        val config = JSONObject()
            .put("activation_key", ActivationStore.load(this)?.key ?: "")
            .put("server_id", serverId)
            .put("server_host", server?.host ?: JSONObject.NULL)
            .toString()
        CoreRegistry.core.start(pfd.detachFd(), config)
        tun = null // fd is detached; nothing to close on our side

        pollJob?.cancel()
        pollJob = scope.launch {
            while (true) {
                val status = Status.fromJson(CoreRegistry.core.statusJson())
                ConnectionRepository.push(status)
                updateNotification(status)
                if (status.state == VpnState.DISCONNECTED) break
                delay(1000)
            }
        }
    }

    private fun disconnect() {
        pollJob?.cancel()
        pollJob = null
        CoreRegistry.core.stop()
        teardownTunnel()
        ConnectionRepository.push(Status())
        stopSelfCleanly()
    }

    private fun teardownTunnel() {
        runCatching { tun?.close() }
        tun = null
    }

    private fun stopSelfCleanly() {
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    override fun onRevoke() {
        // another VPN app took over — mirror a clean disconnect
        disconnect()
    }

    override fun onDestroy() {
        pollJob?.cancel()
        scope.cancel()
        super.onDestroy()
    }

    // ---- notification -------------------------------------------------------

    private fun updateNotification(status: Status) {
        val text = when (status.state) {
            VpnState.CONNECTED -> {
                val s = MockServers.byId(status.serverId)
                getString(R.string.state_connected) +
                    (s?.let { " — ${it.city}, ${it.country}" } ?: "")
            }
            VpnState.CONNECTING -> getString(R.string.state_connecting)
            VpnState.DISCONNECTED -> getString(R.string.state_disconnected)
        }
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIF_ID, buildNotification(text))
    }

    private fun buildNotification(text: String): Notification {
        val nm = getSystemService(NotificationManager::class.java)
        if (nm.getNotificationChannel(CHANNEL_ID) == null) {
            nm.createNotificationChannel(
                NotificationChannel(
                    CHANNEL_ID,
                    getString(R.string.notif_channel_vpn),
                    NotificationManager.IMPORTANCE_LOW,
                ),
            )
        }

        val openApp = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE,
        )
        val disconnect = PendingIntent.getService(
            this, 1,
            disconnectIntent(this),
            PendingIntent.FLAG_IMMUTABLE,
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_tile)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setContentIntent(openApp)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .addAction(0, getString(R.string.action_disconnect), disconnect)
            .build()
    }
}
