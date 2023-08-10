FROM node:20-alpine as build
WORKDIR /usr/src/app
COPY --chown=node:node package.json yarn.lock ./
RUN yarn install --frozen-lockfile
COPY --chown=node:node . .
ENV NODE_ENV production
RUN yarn build
USER node

FROM node:20-alpine as prod
WORKDIR /usr/src/app
COPY --chown=node:node package.json yarn.lock ./
RUN yarn install --production --frozen-lockfile
COPY --from=build /usr/src/app/dist ./dist
COPY ./start.sh ./
EXPOSE 8080
CMD ["/bin/sh", "start.sh"]