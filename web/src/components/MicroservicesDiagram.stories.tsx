import type { Meta, StoryObj } from '@storybook/react';
import { MicroservicesDiagram } from './MicroservicesDiagram';

const meta: Meta<typeof MicroservicesDiagram> = {
  title: 'Components/MicroservicesDiagram',
  component: MicroservicesDiagram,
  parameters: {
    layout: 'fullscreen',
    backgrounds: {
      default: 'dark',
    },
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  decorators: [
    (Story) => (
      <div style={{ width: '100vw', height: '100vh' }}>
        <Story />
      </div>
    ),
  ],
};

export const CompactView: Story = {
  decorators: [
    (Story) => (
      <div style={{ width: '800px', height: '600px', margin: '0 auto' }}>
        <Story />
      </div>
    ),
  ],
};