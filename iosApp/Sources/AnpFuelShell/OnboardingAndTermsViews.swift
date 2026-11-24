import SwiftUI
import AnpFuelCore

/// Terms Gate & 5-Step Onboarding flow synchronized with `main` (P19-T02 / P35).
public struct TermsGateView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @State private var hasReadSummary = false

    private static let termsURL = URL(string: "https://abastevo.com.br/termos/")
    private static let privacyURL = URL(string: "https://abastevo.com.br/privacidade/")

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    HStack(spacing: 12) {
                        Text(String(localized: "onboarding.brand_initial"))
                            .font(.system(size: 28, weight: .black, design: .rounded))
                            .foregroundStyle(AbastevoTheme.accentDark)
                            .frame(width: 48, height: 48)
                            .background(AbastevoTheme.ink)
                            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                        VStack(alignment: .leading, spacing: 2) {
                            Text(String(localized: "brand.name"))
                                .font(.title2.bold())
                            Text(String(localized: "onboarding.tagline"))
                                .font(.subheadline)
                                .foregroundStyle(.secondary)
                        }
                    }

                    EnergiaCard {
                        Text(String(localized: "android.terms_gate_title"))
                            .font(.headline)
                        Text(String(localized: "onboarding.separate_sources"))
                            .font(.body)
                        Label(String(localized: "onboarding.official_separate"), systemImage: "checkmark.seal.fill")
                            .font(.subheadline)
                            .foregroundStyle(.blue)
                        Label(String(localized: "onboarding.no_paid_ranking"), systemImage: "shield.checkered")
                            .font(.subheadline)
                            .foregroundStyle(AbastevoTheme.accentLight)
                        Label(String(localized: "onboarding.location"), systemImage: "location.fill")
                            .font(.subheadline)
                        Label(String(localized: "onboarding.photo_retention"), systemImage: "clock.arrow.circlepath")
                            .font(.subheadline)
                    }

                    EnergiaCard {
                        Text(String(localized: "onboarding.terms_title"))
                            .font(.headline)
                        Text(String(localized: "onboarding.operator_summary"))
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                        if let termsURL = Self.termsURL {
                            Link(String(localized: "android.terms_gate_read_terms"), destination: termsURL)
                                .font(.footnote.bold())
                        }
                        if let privacyURL = Self.privacyURL {
                            Link(String(localized: "android.terms_gate_read_privacy"), destination: privacyURL)
                                .font(.footnote.bold())
                        }
                    }

                    Toggle(String(localized: "android.terms_gate_check"), isOn: $hasReadSummary)
                        .font(.subheadline)
                    Text(String(localized: "android.terms_gate_optional_note"))
                        .font(.footnote)
                        .foregroundStyle(.secondary)

                    Button {
                        IosHaptics.success()
                        viewModel.acceptTerms()
                    } label: {
                        Text(String(localized: "android.terms_gate_accept"))
                            .font(.headline)
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 14)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                    .disabled(!hasReadSummary)
                }
                .padding(20)
            }
            .navigationTitle(String(localized: "onboarding.terms_navigation"))
        }
    }
}

public struct OnboardingFlowView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @State private var step = 0
    @State private var citySearch = ""

    public var body: some View {
        NavigationStack {
            VStack(spacing: 16) {
                ProgressView(value: Double(step + 1), total: 5.0)
                    .tint(AbastevoTheme.accentLight)
                    .padding(.horizontal, 20)
                    .padding(.top, 12)

                TabView(selection: $step) {
                    onboardingCard(
                        icon: "fuelpump.fill",
                        title: String(localized: "android.onboarding_title_welcome"),
                        subtitle: String(localized: "onboarding.compare_sources"),
                        detail: String(localized: "onboarding.national_coverage")
                    )
                    .tag(0)

                    onboardingCard(
                        icon: "arrow.down.circle.fill",
                        title: String(localized: "android.onboarding_title_offline"),
                        subtitle: String(localized: "onboarding.offline_downloads"),
                        detail: String(localized: "onboarding.offline_queue")
                    )
                    .tag(1)

                    onboardingCard(
                        icon: "checkmark.shield.fill",
                        title: String(localized: "android.help_rules_title"),
                        subtitle: String(localized: "onboarding.official_separate"),
                        detail: String(localized: "onboarding.free_account")
                    )
                    .tag(2)

                    onboardingCard(
                        icon: "calendar",
                        title: String(localized: "onboarding.survey_title"),
                        subtitle: String(localized: "onboarding.station_dates"),
                        detail: String(localized: "onboarding.community_prices")
                    )
                    .tag(3)

                    VStack(alignment: .leading, spacing: 12) {
                        Text(String(localized: "android.feed_choose_city"))
                            .font(.title3.bold())

                        TextField(String(localized: "onboarding.city_search"), text: $citySearch)
                            .textFieldStyle(.roundedBorder)

                        let cities = viewModel.municipalityCatalog.search(query: citySearch)
                        if viewModel.municipalityCatalog.count == 0 {
                            Text(String(localized: "onboarding.city_catalog_unavailable"))
                                .foregroundStyle(.secondary)
                        } else if cities.isEmpty {
                            Text(String(localized: "onboarding.city_search_empty"))
                                .foregroundStyle(.secondary)
                        }
                        ScrollView {
                            LazyVStack(spacing: 6) {
                                ForEach(cities) { city in
                                    Button {
                                        viewModel.selectMunicipality(city)
                                    } label: {
                                        HStack {
                                            Text(city.displayTitle)
                                            Spacer()
                                            if viewModel.selectedMunicipality?.ibgeCode == city.ibgeCode {
                                                Image(systemName: "checkmark.circle.fill")
                                                    .foregroundStyle(AbastevoTheme.accentLight)
                                            }
                                        }
                                        .padding(10)
                                        .background(Color.secondary.opacity(0.08))
                                        .clipShape(RoundedRectangle(cornerRadius: 10))
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                        }
                    }
                    .padding(20)
                    .tag(4)
                }
                #if os(iOS)
                .tabViewStyle(.page(indexDisplayMode: .never))
                #endif

                HStack(spacing: 12) {
                    if step > 0 {
                        Button(String(localized: "android.action_back")) {
                            withAnimation { step -= 1 }
                        }
                        .buttonStyle(.bordered)
                    }
                    Spacer()
                    if step < 4 {
                        Button(String(localized: "android.action_next")) {
                            withAnimation { step += 1 }
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.ink)
                    } else {
                        Button(String(localized: "android.onboarding_action_get_started")) {
                            IosHaptics.success()
                            viewModel.completeOnboarding()
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.accentLight)
                    }
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 20)
            }
            .navigationTitle(String(localized: "onboarding.step_count", defaultValue: "Step \((step + 1).formatted()) of 5"))
        }
    }

    private func onboardingCard(icon: String, title: String, subtitle: String, detail: String) -> some View {
        VStack(alignment: .leading, spacing: 16) {
            Spacer()
            Image(systemName: icon)
                .font(.system(size: 46))
                .foregroundStyle(AbastevoTheme.accentLight)
            Text(title)
                .font(.title2.bold())
            Text(subtitle)
                .font(.headline)
                .foregroundStyle(.primary)
            Text(detail)
                .font(.body)
                .foregroundStyle(.secondary)
            Spacer()
        }
        .padding(24)
    }
}

public struct PrivacyDocumentSheet: View {
    @Environment(\.dismiss) private var dismiss

    private static let privacyURL = URL(string: "https://abastevo.com.br/privacidade/")

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text(String(localized: "privacy.policy_title"))
                        .font(.title3.bold())
                    Text(String(localized: "privacy.controller"))
                        .font(.subheadline.bold())
                    Text(String(localized: "privacy.policy_summary"))
                        .font(.body)
                    if let privacyURL = Self.privacyURL {
                        Link(String(localized: "android.privacy_full_policy"), destination: privacyURL)
                    }
                }
                .padding(20)
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }
}
