package com.github.kr328.clash

import com.github.kr328.clash.common.util.ticker
import com.github.kr328.clash.design.ConnectionsDesign
import com.github.kr328.clash.util.withClash
import kotlinx.coroutines.isActive
import kotlinx.coroutines.selects.select
import java.util.concurrent.TimeUnit

class ConnectionsActivity : BaseActivity<ConnectionsDesign>() {
    override suspend fun main() {
        val design = ConnectionsDesign(this)

        setContentDesign(design)

        design.refresh()

        val ticker = ticker(TimeUnit.SECONDS.toMillis(2))

        while (isActive) {
            select<Unit> {
                events.onReceive {
                    when (it) {
                        Event.ClashStop, Event.ServiceRecreated -> finish()
                        else -> Unit
                    }
                }
                design.requests.onReceive {
                    when (it) {
                        ConnectionsDesign.Request.Refresh -> design.refresh()
                        ConnectionsDesign.Request.ClearClosed -> {
                            withClash { clearClosedConnections() }

                            design.refresh()
                        }
                        ConnectionsDesign.Request.CloseAll -> {
                            if (design.requestCloseConfirm(null)) {
                                withClash { closeAllConnections() }

                                design.refresh()
                            }
                        }
                        is ConnectionsDesign.Request.Close -> {
                            if (design.requestCloseConfirm(it.connection)) {
                                withClash { closeConnection(it.connection.id) }

                                design.refresh()
                            }
                        }
                    }
                }
                if (activityStarted) {
                    ticker.onReceive {
                        design.refresh()
                    }
                }
            }
        }
    }

    private suspend fun ConnectionsDesign.refresh() {
        val connections = withClash { queryConnections(filter, sort, activeOnly) }

        show(connections)
    }
}
