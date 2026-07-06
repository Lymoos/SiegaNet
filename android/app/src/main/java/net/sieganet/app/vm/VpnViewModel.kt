package net.sieganet.app.vm

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import net.sieganet.app.activation.ActivationStore
import net.sieganet.app.api.Server
import net.sieganet.app.api.Status
import net.sieganet.app.api.VpnState
import net.sieganet.app.vpn.ConnectionRepository

/**
 * Thin adapter between [ConnectionRepository] (shared with the service and
 * the tile) and the Compose UI, plus the activation state. Owns no VPN
 * logic — actions that touch the tunnel go through VpnService intents fired
 * by the activity.
 */
class VpnViewModel(app: Application) : AndroidViewModel(app) {

    val status: StateFlow<Status> = ConnectionRepository.status
    val servers: StateFlow<List<Server>> = ConnectionRepository.servers
    val selected: StateFlow<Server?> = ConnectionRepository.selected

    // ---- activation (subscription gate) -------------------------------------

    private val _activated = MutableStateFlow(ActivationStore.isActivated(app))
    val activated: StateFlow<Boolean> = _activated.asStateFlow()

    private val _activationBusy = MutableStateFlow(false)
    val activationBusy: StateFlow<Boolean> = _activationBusy.asStateFlow()

    private val _activationError = MutableStateFlow<String?>(null)
    val activationError: StateFlow<String?> = _activationError.asStateFlow()

    /** «подписка до …» for the connect screen subtitle */
    val validUntil: Long?
        get() = ActivationStore.load(getApplication<Application>())?.validUntilUnix

    fun activate(key: String) {
        if (_activationBusy.value) return
        _activationBusy.value = true
        _activationError.value = null
        viewModelScope.launch {
            val result = ActivationStore.verify(getApplication<Application>(), key)
            _activationBusy.value = false
            if (result != null) {
                _activated.value = true
            } else {
                _activationError.value = "Ключ не подошёл. Проверьте и попробуйте ещё раз."
            }
        }
    }

    // ---- servers -------------------------------------------------------------

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
}
