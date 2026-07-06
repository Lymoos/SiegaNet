package net.sieganet.app.vpn

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import net.sieganet.app.api.MockServers
import net.sieganet.app.api.Server
import net.sieganet.app.api.Status

/**
 * Single in-process source of UI truth, shared by the activity, the
 * VpnService, the notification and the quick-settings tile.
 *
 * It carries no VPN logic: [status] is whatever the core reports (the service
 * polls SiegaCore.statusJson() and pushes it here), servers/selection are UI
 * concerns.
 */
object ConnectionRepository {

    private val _status = MutableStateFlow(Status())
    val status: StateFlow<Status> = _status.asStateFlow()

    private val _servers = MutableStateFlow(MockServers.list())
    val servers: StateFlow<List<Server>> = _servers.asStateFlow()

    private val _selected = MutableStateFlow(_servers.value.minByOrNull { it.pingMs })
    val selected: StateFlow<Server?> = _selected.asStateFlow()

    fun push(status: Status) {
        _status.value = status
    }

    fun refreshServers() {
        _servers.value = MockServers.list()
    }

    fun select(server: Server) {
        _selected.value = server
    }
}
