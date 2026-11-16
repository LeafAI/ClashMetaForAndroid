package com.github.kr328.clash.core.model

import android.os.Parcel
import android.os.Parcelable
import com.github.kr328.clash.core.util.Parcelizer
import kotlinx.serialization.Serializable

@Serializable
data class Connection(
    val id: String,
    val network: String,
    val host: String,
    val destinationIP: String,
    val port: Int,
    val process: String,
    val rule: String,
    val rulePayload: String,
    val chains: List<String>,
    val upload: Long,
    val download: Long,
    val start: String,
    val active: Boolean = true,
    val end: String = "",
) : Parcelable {
    val target: String
        get() = host.ifEmpty { destinationIP }

    val ruleLabel: String
        get() = if (rule.isEmpty() || rulePayload.isEmpty()) rule else "$rule($rulePayload)"

    val outbound: String
        get() = chains.firstOrNull().orEmpty()

    override fun writeToParcel(parcel: Parcel, flags: Int) {
        Parcelizer.encodeToParcel(serializer(), parcel, this)
    }

    override fun describeContents(): Int {
        return 0
    }

    companion object CREATOR : Parcelable.Creator<Connection> {
        override fun createFromParcel(parcel: Parcel): Connection {
            return Parcelizer.decodeFromParcel(serializer(), parcel)
        }

        override fun newArray(size: Int): Array<Connection?> {
            return arrayOfNulls(size)
        }
    }
}
