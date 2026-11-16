package com.github.kr328.clash.design

import android.content.Context
import android.text.Editable
import android.text.TextWatcher
import android.view.View
import com.github.kr328.clash.core.model.Connection
import com.github.kr328.clash.core.model.ConnectionSort
import com.github.kr328.clash.design.adapter.ConnectionAdapter
import com.github.kr328.clash.design.databinding.DesignConnectionsBinding
import com.github.kr328.clash.design.util.applyFrom
import com.github.kr328.clash.design.util.applyLinearAdapter
import com.github.kr328.clash.design.util.bindAppBarElevation
import com.github.kr328.clash.design.util.layoutInflater
import com.github.kr328.clash.design.util.root
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlin.coroutines.resume

class ConnectionsDesign(context: Context) : Design<ConnectionsDesign.Request>(context) {
    sealed class Request {
        object Refresh : Request()
        object CloseAll : Request()
        object ClearClosed : Request()

        data class Close(val connection: Connection) : Request()
    }

    private val binding = DesignConnectionsBinding
        .inflate(context.layoutInflater, context.root, false)

    private val adapter = ConnectionAdapter(context) { connection ->
        if (connection.active) {
            requests.trySend(Request.Close(connection))
        }
    }

    override val root: View
        get() = binding.root

    var sort: ConnectionSort = ConnectionSort.Newest
        private set

    val filter: String
        get() = binding.filterView.text.toString()

    val activeOnly: Boolean
        get() = binding.activeOnlyView.isChecked

    suspend fun show(connections: List<Connection>) {
        withContext(Dispatchers.Main) {
            adapter.connections = connections
            adapter.notifyDataSetChanged()

            binding.summaryView.text = context.getString(
                R.string.format_connections_summary,
                connections.count { it.active },
                connections.size,
                context.getString(sortLabel(sort)),
            )
        }
    }

    suspend fun requestCloseConfirm(connection: Connection?): Boolean {
        return withContext(Dispatchers.Main) {
            suspendCancellableCoroutine { ctx ->
                val message = connection?.let { "${it.target}:${it.port}" }
                    ?: context.getString(R.string.close_all_connections_message)

                val dialog = MaterialAlertDialogBuilder(context)
                    .setTitle(if (connection == null) R.string.close_all_connections else R.string.close_connection)
                    .setMessage(message)
                    .setPositiveButton(R.string.ok) { _, _ -> ctx.resume(true) }
                    .setNegativeButton(R.string.cancel) { _, _ -> }
                    .setOnDismissListener { if (!ctx.isCompleted) ctx.resume(false) }
                    .show()

                ctx.invokeOnCancellation { dialog.dismiss() }
            }
        }
    }

    fun requestNextSort() {
        sort = when (sort) {
            ConnectionSort.Newest -> ConnectionSort.Download
            ConnectionSort.Download -> ConnectionSort.Upload
            ConnectionSort.Upload -> ConnectionSort.Newest
        }

        requests.trySend(Request.Refresh)
    }

    fun requestCloseAll() {
        requests.trySend(Request.CloseAll)
    }

    fun requestClearClosed() {
        requests.trySend(Request.ClearClosed)
    }

    private fun sortLabel(sort: ConnectionSort): Int {
        return when (sort) {
            ConnectionSort.Newest -> R.string.connections_sort_newest
            ConnectionSort.Download -> R.string.connections_sort_download
            ConnectionSort.Upload -> R.string.connections_sort_upload
        }
    }

    init {
        binding.self = this

        binding.activityBarLayout.applyFrom(context)

        binding.recyclerList.bindAppBarElevation(binding.activityBarLayout)
        binding.recyclerList.applyLinearAdapter(context, adapter)

        binding.activeOnlyView.setOnCheckedChangeListener { _, _ ->
            requests.trySend(Request.Refresh)
        }

        binding.filterView.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit

            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) = Unit

            override fun afterTextChanged(s: Editable?) {
                requests.trySend(Request.Refresh)
            }
        })
    }
}
