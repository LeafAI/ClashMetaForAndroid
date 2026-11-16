package com.github.kr328.clash.design.adapter

import android.content.Context
import android.text.format.Formatter
import android.view.ViewGroup
import androidx.recyclerview.widget.RecyclerView
import com.github.kr328.clash.core.model.Connection
import com.github.kr328.clash.design.R
import com.github.kr328.clash.design.databinding.AdapterConnectionBinding
import com.github.kr328.clash.design.util.layoutInflater

class ConnectionAdapter(
    private val context: Context,
    private val onLongClick: (Connection) -> Unit,
) : RecyclerView.Adapter<ConnectionAdapter.Holder>() {
    class Holder(val binding: AdapterConnectionBinding) : RecyclerView.ViewHolder(binding.root)

    var connections: List<Connection> = emptyList()

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): Holder {
        return Holder(AdapterConnectionBinding.inflate(context.layoutInflater, parent, false))
    }

    override fun onBindViewHolder(holder: Holder, position: Int) {
        val current = connections[position]

        val traffic = "↑ ${Formatter.formatShortFileSize(context, current.upload)}" +
                "  ↓ ${Formatter.formatShortFileSize(context, current.download)}"

        holder.binding.connection = current
        holder.binding.title = "${current.target}:${current.port}"
        holder.binding.route = "${current.ruleLabel}  →  ${current.outbound}"
        holder.binding.detail = listOf(
            current.network.uppercase(),
            current.process,
            traffic,
            if (current.active) "" else context.getString(R.string.connections_closed),
        )
            .filter { it.isNotEmpty() }
            .joinToString("  ·  ")
        holder.binding.root.alpha = if (current.active) 1f else CLOSED_ALPHA
        holder.binding.root.setOnLongClickListener {
            onLongClick(current)

            true
        }
    }

    override fun getItemCount(): Int {
        return connections.size
    }

    private companion object {
        const val CLOSED_ALPHA = 0.5f
    }
}
