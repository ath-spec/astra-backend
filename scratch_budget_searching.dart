import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../providers/asset_connection_provider.dart';
import '../../chat/widgets/thinking_orbs/thinking_orb.dart';

/// Screen 2 of Banks Flow: Fetching Screen (Image 3) in clean light mode.
/// Displays pulsing dots, skeleton account cards, and auto-navigates to HomeScreen.
class BanksSearchingScreen extends ConsumerStatefulWidget {
  const BanksSearchingScreen({super.key});

  @override
  ConsumerState<BanksSearchingScreen> createState() => _BanksSearchingScreenState();
}

class _BanksSearchingScreenState extends ConsumerState<BanksSearchingScreen>
    with SingleTickerProviderStateMixin {
  Timer? _timer;
  late AnimationController _pulseController;
  late Animation<double> _pulseAnimation;

  @override
  void initState() {
    super.initState();
    _pulseController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 900),
    )..repeat(reverse: true);

    _pulseAnimation = Tween<double>(begin: 0.3, end: 1.0).animate(
      CurvedAnimation(parent: _pulseController, curve: Curves.easeInOut),
    );

    _timer = Timer(const Duration(milliseconds: 2600), () {
      if (mounted) {
        final state = ref.read(assetConnectionProvider);
        if (state.step == AssetConnectionStep.banksLinkingProgress) {
          ref.read(assetConnectionProvider.notifier).completeBankLinking();
        }
        context.pushReplacement('/banks-linking');
      }
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    _pulseController.stop();
    _pulseController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(assetConnectionProvider);
    final isLinking = state.step == AssetConnectionStep.banksLinkingProgress;

    return Scaffold(
      backgroundColor: const Color(0xFFFFFFFF),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(horizontal: 24.0, vertical: 16.0),
                child: Center(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 420),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const SizedBox(height: 24),

                        // Title
                    Text(
                      isLinking ? 'Securely linking your Banks' : 'Securely fetching your Banks',
                      style: const TextStyle(
                        fontFamily: 'SpaceGrotesk',
                        color: Color(0xFF0F172A),
                        fontSize: 28,
                        fontWeight: FontWeight.w700,
                        letterSpacing: -1.0,
                        height: 1.15,
                      ),
                    ),
                    const SizedBox(height: 11),

                    // Subtitle
                    Text(
                      isLinking
                          ? "HANG TIGHT. WE'RE SECURELY LINKING YOUR ACCOUNTS,\nTHIS WILL ONLY TAKE A MOMENT."
                          : "HANG TIGHT. WE'RE SECURELY PULLING YOUR ACCOUNTS,\nTHIS WILL ONLY TAKE A MOMENT.",
                      style: const TextStyle(
                        fontFamily: 'DMSans',
                        color: Color(0xFF9CA3AF),
                        fontSize: 10,
                        height: 1.4,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 36),
                    Center(
                      child: ShaderMask(
                        blendMode: BlendMode.srcIn,
                        shaderCallback: (bounds) => const LinearGradient(
                          colors: [
                            Color(0xFF5BA1F7),
                            Color(0xFF4F46E5),
                          ],
                          begin: Alignment.topLeft,
                          end: Alignment.bottomRight,
                        ).createShader(bounds),
                        child: const ThinkingOrb(
                          mode: 'globe',
                          size: 140,
                          color: Colors.white,
                        ),
                      ),
                    ),
                    const SizedBox(height: 24),
                      ],
                    ),
                  ),
                ),
              ),
            ),

            // Footer
            _buildFooter(),
          ],
        ),
      ),
    );
  }

  Widget _buildFooter() {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 16.0, horizontal: 24.0),
      child: FittedBox(
        fit: BoxFit.scaleDown,
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Text(
              'POWERED BY RBI-REGULATED ACCOUNT AGGREGATOR ',
              style: TextStyle(
                fontFamily: 'DMSans',
                fontSize: 9,
                color: Color(0xFF9CA3AF),
                fontWeight: FontWeight.w600,
              ),
            ),
            const Icon(
              Icons.change_history_rounded,
              color: Color(0xFF1E3A8A),
              size: 11,
            ),
            const SizedBox(width: 2),
            const Text(
              'FINARKEIN',
              style: TextStyle(
                fontFamily: 'DMSans',
                fontSize: 11,
                fontWeight: FontWeight.w800,
                color: Color(0xFF1E3A8A),
                letterSpacing: 0.5,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
