package com.github.kr328.clash.design

import android.content.Context
import android.view.View
import com.github.kr328.clash.design.databinding.DesignRuleSourceBinding
import com.github.kr328.clash.design.util.applyFrom
import com.github.kr328.clash.design.util.layoutInflater
import com.github.kr328.clash.design.util.root
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlin.coroutines.resume

class RuleSourceDesign(
    context: Context,
    private val original: String,
) : Design<RuleSourceDesign.Request>(context) {
    sealed class Request {
        data class Save(val source: String) : Request()
    }

    private val binding = DesignRuleSourceBinding
        .inflate(context.layoutInflater, context.root, false)

    override val root: View
        get() = binding.root

    val source: String
        get() = binding.sourceView.text.toString()

    val modified: Boolean
        get() = source != original

    suspend fun showError(message: String?) {
        withContext(Dispatchers.Main) {
            binding.errorView.text = message
            binding.errorView.visibility = if (message.isNullOrBlank()) View.GONE else View.VISIBLE
        }
    }

    suspend fun requestExitWithoutSaving(): Boolean {
        return withContext(Dispatchers.Main) {
            suspendCancellableCoroutine { ctx ->
                val dialog = MaterialAlertDialogBuilder(context)
                    .setTitle(R.string.exit_without_save)
                    .setMessage(R.string.exit_without_save_warning)
                    .setCancelable(true)
                    .setPositiveButton(R.string.ok) { _, _ -> ctx.resume(true) }
                    .setNegativeButton(R.string.cancel) { _, _ -> }
                    .setOnDismissListener { if (!ctx.isCompleted) ctx.resume(false) }
                    .show()

                ctx.invokeOnCancellation { dialog.dismiss() }
            }
        }
    }

    fun requestSave() {
        requests.trySend(Request.Save(source))
    }

    init {
        binding.self = this

        binding.activityBarLayout.applyFrom(context)

        binding.sourceView.setText(original)
    }
}
