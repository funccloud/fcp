# =================================================================
# Stage 1: Build Stage
# Purpose: Install dependencies and build the Angular application.
# The resulting image from this stage will be large, but it's temporary.
# =================================================================
ARG NODE_VERSION=20.14.0
FROM node:${NODE_VERSION}-alpine AS build

# Set the working directory
WORKDIR /app

# Copy package.json and package-lock.json first to leverage Docker cache
COPY package*.json ./
# Use 'npm ci' for clean, reproducible installs from package-lock.json
RUN npm ci

# Copy the rest of the application source code
COPY . .

# Argument to pass your application name (from angular.json)
# You can change the default 'ui' value here or during the build command
ARG APP_NAME=ui
# Build the application for SSR
# This runs 'ng build' and 'ng run <app-name>:server'
RUN npm run build -- --app-name=${APP_NAME}

# =================================================================
# Stage 2: Production Stage
# Purpose: Run the compiled application in a lightweight Node.js environment.
# This creates a small, optimized final image.
# =================================================================
FROM node:${NODE_VERSION}-alpine

# Set the working directory
WORKDIR /app

# Argument for the application name, must be repeated in this stage
ARG APP_NAME=ui

# Create a non-root user for better security
RUN addgroup -S angular && adduser -S angular -G angular
USER angular

# Copy only the necessary production dependencies manifest
COPY package*.json ./

# Install ONLY production dependencies
RUN npm ci --omit=dev

# Copy the built application from the 'build' stage
# This is the magic of multi-stage builds
COPY --from=build /app/dist/${APP_NAME} ./dist/${APP_NAME}

# Expose the port the server will run on.
# You can change this, but make sure it matches your server.ts config.
EXPOSE 4000

# The command to start the SSR server
CMD [ "node", "dist/ui/server/main.js" ]
