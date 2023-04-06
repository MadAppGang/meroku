FROM node:19

WORKDIR /usr/src/app

COPY package*.json ./
RUN yarn install && yarn cache clean
COPY . .
EXPOSE 8080
CMD ["node", "index.js"]
