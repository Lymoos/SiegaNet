package net.sieganet.app.vm

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import net.sieganet.app.account.AccountStore
import net.sieganet.app.api.Server
import net.sieganet.app.api.Status
import net.sieganet.app.api.VpnState
import net.sieganet.app.vpn.ConnectionRepository

/**
 * Adapter between [ConnectionRepository] (shared with service + tile),
 * [AccountStore] and the Compose UI. Owns no VPN logic — tunnel actions go
 * through VpnService intents fired by the activity.
 */
class VpnViewModel(app: Application) : AndroidViewModel(app) {

    val status: StateFlow<Status> = ConnectionRepository.status
    val servers: StateFlow<List<Server>> = ConnectionRepository.servers
    val selected: StateFlow<Server?> = ConnectionRepository.selected

    // ---- account -------------------------------------------------------------

    private val _account = MutableStateFlow(AccountStore.load(app))
    val account: StateFlow<AccountStore.Account?> = _account.asStateFlow()

    private val _busy = MutableStateFlow(false)
    val busy: StateFlow<Boolean> = _busy.asStateFlow()

    private val _authError = MutableStateFlow<String?>(null)
    val authError: StateFlow<String?> = _authError.asStateFlow()

    val loggedIn: Boolean get() = _account.value != null
    val hasSub: Boolean get() = _account.value?.hasActiveSubscription == true

    fun login(email: String, password: String) {
        if (_busy.value) return
        _busy.value = true
        _authError.value = null
        viewModelScope.launch {
            AccountStore.login(getApplication<Application>(), email, password)
                .onSuccess { _account.value = it }
                .onFailure { _authError.value = it.message ?: "Не удалось войти" }
            _busy.value = false
        }
    }

    /** @param onDone true if the code activated the subscription */
    fun redeem(code: String, onDone: (Boolean) -> Unit = {}) {
        if (_busy.value) return
        _busy.value = true
        _authError.value = null
        viewModelScope.launch {
            AccountStore.redeem(getApplication<Application>(), code)
                .onSuccess { _account.value = it; onDone(true) }
                .onFailure { _authError.value = it.message ?: "Код не подошёл"; onDone(false) }
            _busy.value = false
        }
    }

    fun clearAuthError() {
        _authError.value = null
    }

    fun logout() {
        AccountStore.logout(getApplication<Application>())
        _account.value = null
    }

    // ---- servers -------------------------------------------------------------

    init {
        viewModelScope.launch {
            while (true) {
                delay(8000)
                if (status.value.state != VpnState.CONNECTING) {
                    ConnectionRepository.refreshServers()
                }
            }
        }
    }

    fun select(server: Server) = ConnectionRepository.select(server)
}
