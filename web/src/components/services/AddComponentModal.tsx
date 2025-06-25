import React from 'react';
import { ComponentType } from '../../types';
import { getServiceColor } from '../../utils/colors';
import { Modal, Button } from '../ui';

export interface AddComponentModalProps {
  isOpen: boolean;
  onClose: () => void;
  componentTypes: ComponentType[];
  onAddComponent: (type: string) => void;
}

export const AddComponentModal: React.FC<AddComponentModalProps> = ({
  isOpen,
  onClose,
  componentTypes,
  onAddComponent
}) => {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Add Component"
      footer={
        <Button
          variant="secondary"
          onClick={onClose}
          className="w-full"
        >
          Cancel
        </Button>
      }
    >
      <div className="grid grid-cols-2 gap-3">
        {componentTypes.map((comp) => {
          const color = getServiceColor(comp.type);
          const Icon = comp.icon;
          
          return (
            <button
              key={comp.type}
              onClick={() => onAddComponent(comp.type)}
              className="bg-gray-700 hover:bg-gray-600 p-4 rounded-lg flex flex-col items-center gap-2 transition-all border border-gray-600 hover:border-gray-500"
            >
              <Icon className="w-8 h-8" style={{ color }} />
              <span className="text-gray-200 text-sm">{comp.label}</span>
            </button>
          );
        })}
      </div>
    </Modal>
  );
};