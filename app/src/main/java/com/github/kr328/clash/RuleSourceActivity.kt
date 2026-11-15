package com.github.kr328.clash

import com.github.kr328.clash.design.R
import com.github.kr328.clash.design.RuleSourceDesign
import com.github.kr328.clash.design.ui.ToastDuration
import com.github.kr328.clash.util.withClash
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.selects.select

class RuleSourceActivity : BaseActivity<RuleSourceDesign>() {
    override suspend fun main() {
        val original = withClash { readRuleSource() }
        val design = RuleSourceDesign(this, original)

        setContentDesign(design)

        while (isActive) {
            select<Unit> {
                events.onReceive {
                    if (it == Event.ServiceRecreated) {
                        finish()
                    }
                }
                design.requests.onReceive {
                    when (it) {
                        is RuleSourceDesign.Request.Save -> design.save(it.source)
                    }
                }
            }
        }
    }

    private suspend fun RuleSourceDesign.save(source: String) {
        val error = withClash { writeRuleSource(source) }

        if (error != null) {
            showError(error)

            return
        }

        showError(null)
        showToast(R.string.rule_source_saved, ToastDuration.Short)

        finish()
    }

    override fun onBackPressed() {
        design?.apply {
            launch {
                if (!modified || requestExitWithoutSaving()) {
                    finish()
                }
            }
        } ?: return super.onBackPressed()
    }
}
