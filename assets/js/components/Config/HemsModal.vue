<template>
	<DeviceModalBase
		:id="id"
		ref="deviceModal"
		name="hems"
		device-type="hems"
		:modal-title="$t('config.hems.title')"
		:provide-template-options="provideTemplateOptions"
		:initial-values="initialValues"
		:is-yaml-input-type="isYamlInputType"
		:on-template-change="handleTemplateChange"
		:hide-template-fields="fromYaml"
		:show-main-content="activeTab === 'dynamic'"
		:hide-delete="true"
		:hide-info="true"
		:keep-open-on-remove="true"
		@added="onAdded"
		@updated="onUpdated"
		@removed="onRemoved"
		@open="onOpen"
		@close="$emit('close')"
	>
		<template v-if="id !== undefined && !fromYaml" #template-action>
			<button
				type="button"
				class="btn btn-outline-secondary border-0"
				:aria-label="$t('config.general.change')"
				:title="$t('config.general.change')"
				:disabled="changing"
				@click="handleChange"
			>
				<shopicon-regular-edit size="s" class="flex-shrink-0"></shopicon-regular-edit>
			</button>
		</template>
		<template #pre-content>
			<p class="mt-0 mb-4">
				{{ $t("config.hems.description") }}
				<a :href="docsLink" target="_blank" rel="noopener">
					{{ $t("config.general.docsLink") }}
				</a>
			</p>
			<ul class="nav nav-tabs mb-4">
				<li class="nav-item">
					<a
						class="nav-link"
						:class="{ active: activeTab === 'dynamic' }"
						href="#"
						@click.prevent="activeTab = 'dynamic'"
					>
						{{ $t("config.hems.dynamicLimits") }}
					</a>
				</li>
				<li class="nav-item">
					<a
						class="nav-link"
						:class="{ active: activeTab === 'static' }"
						href="#"
						@click.prevent="activeTab = 'static'"
					>
						{{ $t("config.hems.staticLimits") }}
					</a>
				</li>
			</ul>
		</template>
		<template #description>
			<div v-if="configured" class="mb-4" data-testid="grid-sessions">
				<h6 class="mb-3">{{ $t("config.hems.recordedEvents") }}</h6>
				<div class="events-box rounded p-3">
					<p v-if="!sessionCount" class="mb-0 text-muted">
						{{ $t("config.hems.noEvents") }}
					</p>
					<template v-else>
						<p class="mb-3">
							{{ $t("config.hems.eventsRecorded", { count: sessionCount }) }}<br />
							<template v-if="lastEventTimeAgo">
								{{ $t("config.hems.lastEvent", { timeAgo: lastEventTimeAgo }) }}
							</template>
						</p>
						<DownloadButton :label="$t('general.download')" :href="downloadHref()" />
					</template>
				</div>
				<hr class="mt-4 mb-0" />
			</div>
			<p v-if="fromYaml" class="text-muted">
				{{ $t("config.general.fromYamlHint") }}
			</p>
		</template>
		<template v-if="activeTab === 'static'" #post-content>
			<div v-if="experimental" class="mb-4" data-testid="grid-export-limit">
				<div class="form-check form-switch">
					<input
						id="hemsExportLimitEnabled"
						class="form-check-input"
						type="checkbox"
						role="switch"
						:checked="exportLimitEnabled"
						:disabled="exportLimitState === 'saving'"
						@change="toggleExportLimit"
					/>
					<label for="hemsExportLimitEnabled" class="form-check-label">
						{{ $t("config.hems.exportLimit") }} 🧪
					</label>
					<div class="ps-2">
						<p class="text-muted small mb-2">
							{{ $t("config.hems.exportLimitHint") }}
						</p>
						<div class="collapsible-wrapper" :class="{ open: exportLimitEnabled }">
							<div class="collapsible-content ring-space">
								<div class="d-flex align-items-center gap-2">
									<div class="input-group input-width">
										<input
											id="hemsExportLimit"
											v-model.number="exportLimit"
											type="number"
											min="0"
											:aria-label="$t('config.hems.exportLimit')"
											aria-describedby="hemsExportLimitUnit"
											class="form-control text-end"
											@input="onExportLimitInput"
											@blur="commitExportLimit"
											@keydown.enter.prevent="commitExportLimit"
										/>
										<span id="hemsExportLimitUnit" class="input-group-text">
											W
										</span>
									</div>
									<SavingIndicator :state="exportLimitState" />
								</div>
								<div class="form-text evcc-gray mb-2">
									{{ $t("config.hems.exportLimitDescription") }}
								</div>
							</div>
						</div>
					</div>
				</div>
			</div>
			<div data-testid="peak-limit">
				<div class="form-check form-switch">
					<input
						id="hemsPeakEnabled"
						class="form-check-input"
						type="checkbox"
						role="switch"
						:checked="peakEnabled"
						:disabled="peakState === 'saving'"
						@change="togglePeak"
					/>
					<label for="hemsPeakEnabled" class="form-check-label">
						{{ $t("config.hems.peakLimit") }}
					</label>
					<div class="ps-2">
						<p class="text-muted small mb-2">
							{{ $t("config.hems.peakLimitHint") }}
						</p>
						<div class="collapsible-wrapper" :class="{ open: peakEnabled }">
							<div class="collapsible-content ring-space">
								<div class="d-flex align-items-center gap-2">
									<div class="input-group input-width">
										<input
											id="hemsPeakLimit"
											v-model.number="peakLimit"
											type="number"
											min="0"
											step="0.1"
											:aria-label="$t('config.hems.peakLimit')"
											aria-describedby="hemsPeakLimitUnit"
											class="form-control text-end"
											@input="onPeakLimitInput"
											@blur="commitPeak"
											@keydown.enter.prevent="commitPeak"
										/>
										<span id="hemsPeakLimitUnit" class="input-group-text">
											kW
										</span>
									</div>
									<SavingIndicator :state="peakState" />
								</div>
								<div class="form-text evcc-gray mb-2">
									{{ $t("config.hems.peakLimitDescription") }}
								</div>
								<dl v-if="peakStatus" class="peak-status small mb-2">
									<dt>{{ $t("config.hems.peakMonth") }}</dt>
									<dd data-testid="peak-month">{{ peakMonthFmt }}</dd>
									<dt>{{ $t("config.hems.peakQuarterHour") }}</dt>
									<dd
										data-testid="peak-quarter-hour"
										:class="{ 'text-warning': peakQuarterHourAbove }"
									>
										{{ peakProjectedFmt }}
									</dd>
								</dl>
								<p v-if="peakStatus?.exceeded" class="small text-warning mb-2">
									{{ $t("config.hems.peakExceeded", { target: peakTargetFmt }) }}
								</p>
							</div>
						</div>
					</div>
				</div>
			</div>
		</template>
	</DeviceModalBase>
</template>

<script lang="ts">
import { defineComponent, type PropType } from "vue";
import "@h2d2/shopicons/es/regular/edit";
import DeviceModalBase from "./DeviceModal/DeviceModalBase.vue";
import { ConfigType, type PeakStatus, type YamlSource } from "@/types/evcc";
import { type DeviceValues } from "./DeviceModal";
import { customTemplateOption, type TemplateGroup } from "./DeviceModal/TemplateSelector.vue";
import customHemsYaml from "./defaultYaml/customHems.yaml?raw";
import relayHemsYaml from "./defaultYaml/relayHems.yaml?raw";
import api from "../../api";
import store from "@/store";
import { docsPrefix } from "@/i18n";
import DownloadButton from "../Helper/DownloadButton.vue";
import SavingIndicator, { type SavingState } from "../Helper/SavingIndicator.vue";
import formatter, { POWER_UNIT } from "../../mixins/formatter";

// selector value for the relay variant; both variants save as type custom
const RELAY_OPTION = "relay";

let exportLimitTimer: ReturnType<typeof setTimeout> | undefined;
let peakLimitTimer: ReturnType<typeof setTimeout> | undefined;

const initialValues = {
	type: ConfigType.Template,
	icon: undefined,
	deviceProduct: undefined,
	yaml: undefined,
	template: null,
};

export default defineComponent({
	name: "HemsModal",
	components: { DeviceModalBase, DownloadButton, SavingIndicator },
	mixins: [formatter],
	props: {
		yamlSource: String as PropType<YamlSource>,
		id: Number as PropType<number | undefined>,
	},
	emits: ["changed", "close"],
	data() {
		return {
			initialValues,
			sessions: [] as Array<{ created: string }>,
			changing: false,
			exportLimit: null as number | null,
			exportLimitEnabled: false,
			exportLimitState: null as SavingState,
			peakLimit: null as number | null,
			peakEnabled: false,
			peakState: null as SavingState,
			activeTab: "dynamic" as "dynamic" | "static",
		};
	},
	computed: {
		fromYaml(): boolean {
			return this.yamlSource === "file";
		},
		configured(): boolean {
			return this.id !== undefined || this.fromYaml;
		},
		sessionCount(): number {
			return this.sessions.length;
		},
		lastEvent() {
			return this.sessions[0] ?? null;
		},
		lastEventTimeAgo(): string {
			const created = this.lastEvent?.created;
			if (!created) return "";
			const ms = new Date(created).getTime();
			if (!Number.isFinite(ms)) return "";
			return (this as any).fmtTimeAgo(ms - Date.now());
		},
		docsLink(): string {
			return `${docsPrefix()}/external-limit`;
		},
		experimental(): boolean {
			return store.state?.experimental === true;
		},
		serverExportLimit(): number {
			return store.state?.gridExportLimit || 0;
		},
		exportLimitUnchanged(): boolean {
			return (this.exportLimit || 0) === this.serverExportLimit;
		},
		serverPeakFollowing(): boolean {
			return store.state?.peakFollowing === true;
		},
		serverPeakLimit(): number {
			return store.state?.peakLimit || 0;
		},
		peakLimitW(): number {
			return Math.round((this.peakLimit || 0) * 1e3);
		},
		peakUnchanged(): boolean {
			return (
				this.peakLimitW === this.serverPeakLimit &&
				this.peakEnabled === this.serverPeakFollowing
			);
		},
		peakStatus(): PeakStatus | null {
			return store.state?.peak ?? null;
		},
		peakMonthFmt(): string {
			const { monthPeak = 0, monthPeakTime } = this.peakStatus || {};
			const power = this.fmtW(monthPeak, POWER_UNIT.KW);
			if (!monthPeakTime) return power;
			return `${power} (${this.fmtWeekdayDayTime(new Date(monthPeakTime))})`;
		},
		peakProjectedFmt(): string {
			return this.fmtW(this.peakStatus?.projected || 0, POWER_UNIT.KW);
		},
		peakTargetFmt(): string {
			return this.fmtW(this.peakStatus?.target || 0, POWER_UNIT.KW);
		},
		peakQuarterHourAbove(): boolean {
			const { projected = 0, target = 0 } = this.peakStatus || {};
			return projected > target;
		},
	},
	beforeUnmount() {
		clearTimeout(exportLimitTimer);
		clearTimeout(peakLimitTimer);
	},
	methods: {
		onOpen() {
			this.exportLimit = this.serverExportLimit || null;
			this.exportLimitEnabled = this.serverExportLimit > 0;
			this.exportLimitState = null;
			this.peakLimit = this.serverPeakLimit ? this.serverPeakLimit / 1e3 : null;
			this.peakEnabled = this.serverPeakFollowing;
			this.peakState = null;
			this.activeTab = "dynamic";
			this.loadSessions();
		},
		onExportLimitInput() {
			clearTimeout(exportLimitTimer);
			exportLimitTimer = setTimeout(this.commitExportLimit, 2000);
		},
		commitExportLimit() {
			clearTimeout(exportLimitTimer);
			if (this.exportLimitState === "saving" || this.exportLimitUnchanged) return;
			this.postExportLimit(this.exportLimit || 0);
		},
		toggleExportLimit($event: Event) {
			const { checked } = $event.target as HTMLInputElement;
			this.exportLimitEnabled = checked;
			this.exportLimitState = null;
			if (!checked) {
				this.exportLimit = null;
				this.commitExportLimit();
			}
		},
		async postExportLimit(value: number) {
			this.exportLimitState = "saving";
			try {
				await api.post(`gridexportlimit/${value}`);
				this.exportLimitState = "saved";
			} catch (e) {
				this.exportLimitState = null;
				console.error(e);
			}
		},
		onPeakLimitInput() {
			clearTimeout(peakLimitTimer);
			peakLimitTimer = setTimeout(this.commitPeak, 2000);
		},
		togglePeak($event: Event) {
			const { checked } = $event.target as HTMLInputElement;
			this.peakEnabled = checked;
			this.peakState = null;
			this.commitPeak();
		},
		commitPeak() {
			clearTimeout(peakLimitTimer);
			if (this.peakState === "saving" || this.peakUnchanged) return;
			// enabling waits for a limit
			if (this.peakEnabled && this.peakLimitW <= 0) return;
			this.postPeak();
		},
		async postPeak() {
			this.peakState = "saving";
			try {
				// limit must be set before enabling and may only be cleared after disabling
				if (this.peakEnabled) {
					await api.post(`peaklimit/${this.peakLimitW}`);
					await api.post(`peakfollowing/true`);
				} else {
					await api.post(`peakfollowing/false`);
				}
				this.peakState = "saved";
			} catch (e) {
				this.peakState = null;
				console.error(e);
			}
		},
		downloadHref(): string {
			const params = new URLSearchParams({ lang: this.$i18n?.locale });
			return `./api/gridsessions?${params.toString()}`;
		},
		async loadSessions() {
			try {
				const response = await api.get("gridsessions", {
					validateStatus: (code: number) => [200, 404].includes(code),
				});
				this.sessions = response.data || [];
			} catch (e) {
				this.sessions = [];
				console.error(e);
			}
		},
		provideTemplateOptions(products: any[]): TemplateGroup[] {
			if (this.fromYaml) {
				return [];
			}
			return [
				{
					label: "generic",
					options: [
						customTemplateOption(this.$t("config.hems.type.custom")),
						customTemplateOption(this.$t("config.hems.type.relay"), RELAY_OPTION),
					],
				},
				{
					label: "integrations",
					options: products,
				},
			];
		},
		isYamlInputType(type: ConfigType): boolean {
			return type === ConfigType.Custom || (type as string) === RELAY_OPTION;
		},
		handleTemplateChange(value: string, values: DeviceValues) {
			if (this.isYamlInputType(value as ConfigType)) {
				values.type = ConfigType.Custom;
				values.yaml = value === RELAY_OPTION ? relayHemsYaml : customHemsYaml;
			}
		},
		onAdded(name: string) {
			this.$emit("changed", { action: "added", name });
		},
		onUpdated() {
			this.$emit("changed", { action: "updated" });
		},
		onRemoved() {
			this.$emit("changed", { action: "removed" });
		},
		async handleChange() {
			if (this.id === undefined) return;
			if (!window.confirm(this.$t("config.hems.changeConfirm"))) return;
			this.changing = true;
			try {
				await (this.$refs["deviceModal"] as any).remove();
			} finally {
				this.changing = false;
			}
		},
	},
});
</script>

<style scoped>
.events-box {
	background: var(--evcc-background);
}
.input-width {
	max-width: 180px;
}
.peak-status {
	display: grid;
	grid-template-columns: auto 1fr;
	column-gap: 1rem;
}
.peak-status dd {
	margin-bottom: 0;
}
</style>
