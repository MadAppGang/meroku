import type { Meta, StoryObj } from '@storybook/react';
import { DeploymentPlatform } from './DeploymentPlatform';

const meta: Meta<typeof DeploymentPlatform> = {
  title: 'Components/DeploymentPlatform',
  component: DeploymentPlatform,
  parameters: {
    layout: 'fullscreen',
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  decorators: [
    (Story) => (
      <div style={{ height: '100vh' }}>
        <Story />
      </div>
    ),
  ],
};