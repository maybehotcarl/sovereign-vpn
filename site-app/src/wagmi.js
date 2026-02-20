import { getDefaultConfig } from '@rainbow-me/rainbowkit';
import { mainnet, sepolia } from 'wagmi/chains';

const chains = import.meta.env.VITE_CHAIN === 'sepolia' ? [sepolia] : [mainnet];

export const config = getDefaultConfig({
  appName: '6529 VPN',
  // Get a free project ID at https://cloud.walletconnect.com
  projectId: 'f5a95088cbbb27fd501c70138334ed22',
  chains,
  ssr: false,
});
