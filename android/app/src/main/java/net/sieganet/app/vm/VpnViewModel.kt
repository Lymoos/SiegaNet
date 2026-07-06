package net.sieganet.app.vm

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import net.sieganet.app.api.Server
import net.sieganet.app.api.Status
import net.sieganet.app.api.VpnState
import net.sieganet.app.config.ConfigStore
import net.sieganet.app.vpn.ConnectionRepository

/**
 * Thin adapter between [ConnectionRepository] (shared with the service and
 * the tile) and the Compose UI. Owns no VPN logic — actions that touch the
 * tunnel go through VpnService intents fired by the activity.
 */
class VpnViewModel(app: Application) : AndroidViewModel(app) {

    val status: StateFlow<Status> = ConnectionRepository.status
    val servers: StateFlow<List<Server>> = ConnectionRepository.servers
    val selected: StateFlow<Server?> = ConnectionRepository.selected

    private val _configSummary = MutableStateFlow(ConfigStore.summary(app))
    val configSummary: StateFlow<String?> = _configSummary.asStateFlow()

    init {
        // ping jitter / load drift while the app is open
        viewModelScope.launch {
            while (true) {
                delay(8000)
                // don't reshuffle mid-connection state transitions
                if (status.value.state != VpnState.CONNECTING) {
                    ConnectionRepository.refreshServers()
                }
            }
        }
    }

    fun select(server: Server) = ConnectionRepository.select(server)

    /** @return human summary on success, null if the payload is unparseable */
    fun importConfig(text: String): String? {
        val summary = ConfigStore.importFrom(getApplication<Application>(), text)
        if (summary != null) _configSummary.value = summary
        return summary
    }
}
