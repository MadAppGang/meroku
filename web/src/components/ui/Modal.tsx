import React from 'react';
import { X } from 'lucide-react';
import { IconButton } from './IconButton';

export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  children,
  footer
}) => {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-gray-800 rounded-xl p-6 w-96 shadow-2xl">
        {title && (
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-white text-xl font-semibold">{title}</h2>
            <IconButton
              icon={<X className="w-5 h-5" />}
              onClick={onClose}
            />
          </div>
        )}
        <div>{children}</div>
        {footer && (
          <div className="mt-4">{footer}</div>
        )}
      </div>
    </div>
  );
};