import { getDefaultConfig } from '@rainbow-me/rainbowkit';
import { mainnet } from 'wagmi/chains';

export const config = getDefaultConfig({
  appName: '6529 VPN',
  // Get a free project ID at https://cloud.walletconnect.com
  projectId: 'f5a95088cbbb27fd501c70138334ed22',
  chains: [mainnet],
  ssr: false,
});
