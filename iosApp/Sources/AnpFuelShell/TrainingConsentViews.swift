import AnpFuelCore
import SwiftUI

public struct TrainingConsentSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.locale) private var locale
    @State private var showReader = false
    let model: TrainingConsentModel
    let onSignIn: () -> Void

    public var body: some View {
        NavigationStack {
            List {
                Section {
                    Text(String(localized: "android.privacy_training_unavailable"))
                    Text(String(localized: "android.privacy_training_optional"))
                }
                documentSection
                preferenceSection
                Section(String(localized: "android.privacy_rights_heading")) {
                    Text(String(localized: "android.privacy_rights_body"))
                        .textSelection(.enabled)
                }
            }
            .navigationTitle(String(localized: "privacy.ocr_training"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
            .task(id: model.reviewID) { model.refresh() }
            .onChange(of: model.reviewID) { _, _ in showReader = false }
            .sheet(isPresented: $showReader) {
                TrainingTermsReader(model: model)
                    .id(model.reviewID)
            }
        }
    }

    private var documentSection: some View {
        Section(String(localized: "privacy.consent_document")) {
            if TrainingTermsContract.verifiedDigestMatches() {
                Text(String(localized: "android.privacy_terms_language"))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                Button(String(localized: "android.privacy_read_terms")) { showReader = true }
            } else {
                Text(String(localized: "android.privacy_terms_unavailable"))
            }
        }
    }

    @ViewBuilder
    private var preferenceSection: some View {
        Section {
            if model.hasOwner {
                preferenceStatus
                if !model.confirmedGrant, !model.termsSuperseded, !model.isPendingRevocation,
                   TrainingTermsContract.verifiedDigestMatches() {
                    if model.review?.read == true {
                        Toggle(String(localized: "android.privacy_consent_check"), isOn: Binding(
                            get: { model.review?.checked == true },
                            set: { model.checkTerms($0) }
                        ))
                        .disabled(!model.canGrant)
                    }
                    if model.review?.canConfirm == true {
                        Button(String(localized: "android.privacy_confirm_preference")) { model.grant() }
                            .disabled(!model.canGrant)
                            .fontWeight(.semibold)
                    }
                }
                Button(String(localized: "android.privacy_revoke_delete"), role: .destructive) { model.revoke() }
                    .disabled(model.isPendingRevocation)
                Text(String(localized: "android.privacy_deletion_receipt_note"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                if !model.isOffline, model.refusal != nil || model.isPendingRevocation {
                    Button(String(localized: "android.action_retry")) { model.refresh() }
                        .disabled(model.isBusy)
                }
            } else {
                Button(String(localized: "android.privacy_sign_in"), action: onSignIn)
            }
        }
    }

    @ViewBuilder
    private var preferenceStatus: some View {
        if model.isPendingRevocation {
            Text(String(localized: "android.privacy_revocation_pending"))
        } else if model.isOffline {
            Text(String(localized: "android.privacy_offline"))
        } else if model.confirmedGrant {
            Text(String(localized: "android.privacy_preference_received"))
            if let version = model.status?.termsVersion ?? model.receipt?.termsVersion,
               let date = model.status?.decidedAt ?? model.receipt?.decidedAt {
                Text(String(
                    localized: "android.privacy_recorded_details",
                    defaultValue: "Terms \(version) · decision recorded by the server on \(date.formatted(.dateTime.year().month().day().hour().minute().locale(locale)))"
                ))
                .font(.footnote)
                .foregroundStyle(.secondary)
            }
        } else if model.receipt?.state == .revoked {
            Text(String(localized: "privacy.revocation_received"))
        } else if model.status != nil {
            Text(String(localized: "android.privacy_preference_off"))
        }
        if model.status?.needsRegrant == true || model.termsSuperseded {
            Text(String(localized: "android.training_consent_regrant"))
        }
        if let refusal = model.refusal, refusal != .termsOutdated {
            Text(message(for: refusal))
                .foregroundStyle(.secondary)
        }
        if model.isBusy {
            ProgressView()
                .accessibilityLabel(String(localized: "privacy.checking_preference"))
        }
    }

    private func message(for refusal: TrainingConsentError) -> String {
        switch refusal {
        case .signInRequired: return String(localized: "android.privacy_sign_in")
        case .termsOutdated: return String(localized: "android.training_consent_regrant")
        case .staleRevision, .conflict: return String(localized: "privacy.decision_conflict")
        case .invalid: return String(localized: "privacy.decision_invalid")
        case .offline: return String(localized: "android.privacy_offline")
        case .unavailable: return String(localized: "android.privacy_request_failed")
        case .localStorage: return String(localized: "privacy.decision_storage_failed")
        }
    }
}

private struct TrainingTermsReader: View {
    @Environment(\.dismiss) private var dismiss
    @State private var checked = false
    let model: TrainingConsentModel

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    Text(verbatim: TrainingTermsContract.fullTextPtBr)
                        .textSelection(.enabled)
                    Text(String(
                        localized: "android.privacy_digest",
                        defaultValue: "Document SHA-256: \(TrainingTermsContract.digestSha256)"
                    ))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    if model.canGrant {
                        Text(String(localized: "android.privacy_modal_hint"))
                            .font(.subheadline)
                        Toggle(String(localized: "android.privacy_consent_check"), isOn: $checked)
                        Button(String(localized: "android.privacy_accept_record"), action: confirm)
                            .disabled(!checked)
                            .buttonStyle(.borderedProminent)
                    }
                    Button(String(localized: "android.privacy_finish_reading")) {
                        model.finishReading()
                        dismiss()
                    }
                    .buttonStyle(.bordered)
                }
                .padding()
                .frame(maxWidth: 680)
                .frame(maxWidth: .infinity)
            }
            .navigationTitle(String(localized: "android.privacy_read_terms"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }

    private func confirm() {
        guard checked, model.canGrant else { return }
        model.finishReading()
        model.checkTerms(true)
        model.grant()
        dismiss()
    }
}
